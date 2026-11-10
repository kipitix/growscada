package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// DiscardService is the Engineering node's Discard and Revert (ADR 0016,
// task 72_4): Discard resets the whole Draft to the Project's last Commit —
// the saved state, dropping unsaved work —, while Revert resets it to the
// Deployment the Project runs. Each is one transaction over the Draft.
type DiscardService interface {
	// Discard resets the live Project projectID's Draft to its last Commit,
	// dropping the unsaved changes. The written Draft is one change: its
	// content is replaced, its DraftChange stack is cleared and its Version
	// raised. A Library Pin the Commit names but this server cannot write is
	// dropped with the Widgets of its WidgetTypes, and reported in the answer's
	// warnings: the Draft stays writable (task 71).
	// Returns ErrNothingToDiscard if the Project has no Commit, and an error
	// wrapping project.ErrProjectNotFound if it is not live.
	Discard(ctx context.Context, projectID uuid.UUID) (appdto.DiscardedDraft, error)
	// Revert resets the Draft to the Deployment the Project runs, as Discard
	// did before task 72_4. It refuses with ErrNoDeploymentToDiscard when the
	// Project runs none.
	Revert(ctx context.Context, projectID uuid.UUID) (appdto.DiscardedDraft, error)
}

type discardServiceImpl struct {
	content   ProjectContent
	runtime   RuntimeNode
	libraries library.LibraryRepository
	releases  library.LibraryReleaseRepository
}

var _ DiscardService = (*discardServiceImpl)(nil)

// NewDiscardService returns the DiscardService of the live Projects the
// ProjectContent serves, reading the current Deployment through the Runtime node
// aRuntime and checking its Library Pins against this server's Libraries and
// Releases (ADR 0018, task 71).
func NewDiscardService(aContent ProjectContent, aRuntime RuntimeNode,
	someLibraries library.LibraryRepository, someReleases library.LibraryReleaseRepository) DiscardService {
	return &discardServiceImpl{content: aContent, runtime: aRuntime, libraries: someLibraries, releases: someReleases}
}

func (s discardServiceImpl) Discard(ctx context.Context, rawProjectID uuid.UUID) (appdto.DiscardedDraft, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.DiscardedDraft{}, err
	}
	// The newest Commit is what the Draft falls back to when its unsaved
	// changes go.
	last, found, err := s.content.Drafts().FindLastCommit(ctx, s.content.ScopeOf(projectID))
	if err != nil {
		return appdto.DiscardedDraft{}, fmt.Errorf("cannot read the last commit of project %s: %w", projectID, err)
	}
	if !found {
		return appdto.DiscardedDraft{}, fmt.Errorf("%w: %s", ErrNothingToDiscard, projectID)
	}
	file, ok := last.Content().ProjectFile()
	if !ok {
		return appdto.DiscardedDraft{}, fmt.Errorf("commit %d of project %s holds no project file", last.Number(), projectID)
	}
	content, err := contentOfProjectFile(projectID, file)
	if err != nil {
		return appdto.DiscardedDraft{}, fmt.Errorf("cannot read commit %d of project %s: %w", last.Number(), projectID, err)
	}
	missing, err := missingLibraryReleases(ctx, s.libraries, s.releases, file)
	if err != nil {
		return appdto.DiscardedDraft{}, fmt.Errorf("cannot check the library pins of project %s: %w", projectID, err)
	}
	content, warnings := dropMissingLibraryReleases(content, file, missing)
	result, err := s.content.Drafts().Replace(ctx, s.content.ScopeOf(projectID), len(warnings) == 0, func(session *draft.Session) error {
		if err := s.lockDiscardedPins(ctx, content, session); err != nil {
			return err
		}
		return session.Replace(ctx, content)
	})
	if err != nil {
		return appdto.DiscardedDraft{}, fmt.Errorf("cannot discard project %s: %w", projectID, err)
	}
	return appdto.DiscardedDraft{Version: result.Version().Number(), Warnings: warnings}, nil
}

func (s discardServiceImpl) Revert(ctx context.Context, rawProjectID uuid.UUID) (appdto.DiscardedDraft, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.DiscardedDraft{}, err
	}
	deployment, ok, err := s.runtime.FindCurrentDeployment(ctx, projectID.UUID())
	if err != nil {
		return appdto.DiscardedDraft{}, fmt.Errorf("cannot read the current deployment of project %s: %w", projectID, err)
	}
	if !ok {
		return appdto.DiscardedDraft{}, fmt.Errorf("%w: %s", ErrNoDeploymentToDiscard, projectID)
	}
	content, err := contentOfProjectFile(projectID, deployment.Content)
	if err != nil {
		return appdto.DiscardedDraft{}, fmt.Errorf("cannot read the deployment of project %s: %w", projectID, err)
	}
	// A Deployment deployed from a foreign file, or one whose Library was
	// removed after it, may name Library Releases this server does not have.
	// The Draft cannot hold such a pin: it goes with the Widgets of its
	// Library's WidgetTypes, and the answer names it — rather than refusing the
	// Revert and leaving the Draft unwritable (task 71).
	missing, err := missingLibraryReleases(ctx, s.libraries, s.releases, deployment.Content)
	if err != nil {
		return appdto.DiscardedDraft{}, fmt.Errorf("cannot check the library pins of project %s: %w", projectID, err)
	}
	content, warnings := dropMissingLibraryReleases(content, deployment.Content, missing)
	// The Draft the Revert writes is its last Commit's content only when the
	// Deployment is exactly that and nothing of it was dropped: then it is
	// clean, as after a Discard. A Deployment of another Release, of a file or
	// of an older state is a different content (task 74).
	clean, err := s.revertIsTheLastCommit(ctx, projectID, deployment.Content)
	if err != nil {
		return appdto.DiscardedDraft{}, err
	}
	clean = clean && len(warnings) == 0
	result, err := s.content.Drafts().Replace(ctx, s.content.ScopeOf(projectID), clean, func(session *draft.Session) error {
		if err := s.lockDiscardedPins(ctx, content, session); err != nil {
			return err
		}
		return session.Replace(ctx, content)
	})
	if err != nil {
		return appdto.DiscardedDraft{}, fmt.Errorf("cannot revert project %s: %w", projectID, err)
	}
	return appdto.DiscardedDraft{Version: result.Version().Number(), Warnings: warnings}, nil
}

// revertIsTheLastCommit reports whether the document of the Deployment a
// Project runs is the content of its last Commit: the Draft a Revert writes is
// then that Commit again and clean, as after a Discard (task 74). A Project
// with no Commit, or a Deployment of another content, is not clean.
func (s discardServiceImpl) revertIsTheLastCommit(ctx context.Context, projectID id.ID[project.Project], document projectv0.ProjectFile) (bool, error) {
	last, found, err := s.content.Drafts().FindLastCommit(ctx, s.content.ScopeOf(projectID))
	if err != nil {
		return false, fmt.Errorf("cannot read the last commit of project %s: %w", projectID, err)
	}
	if !found {
		return false, nil
	}
	file, ok := last.Content().ProjectFile()
	if !ok {
		return false, nil
	}
	same, err := sameProjectFile(document, file)
	if err != nil {
		return false, fmt.Errorf("cannot compare the deployment and the last commit of project %s: %w", projectID, err)
	}
	return same, nil
}

// lockDiscardedPins locks every Library the kept pins name FOR SHARE in the
// Discard's or Revert's transaction: a Library deleted between the check and the
// write makes the command refuse, not fail on the foreign key (ADR 0018). The
// pins the check already dropped are not locked.
func (s discardServiceImpl) lockDiscardedPins(ctx context.Context, content draft.Content, session *draft.Session) error {
	var issues []FileIssue
	for i, pin := range content.LibraryPins {
		if err := session.LockLibrary(ctx, pin.LibraryID()); err != nil {
			if errors.Is(err, library.ErrLibraryNotFound) {
				issues = append(issues, FileIssue{
					Path:    fmt.Sprintf("library_pins[%d]", i),
					Message: fmt.Sprintf("library %s is not live on this server", pin.LibraryID()),
				})
				continue
			}
			return err
		}
	}
	if len(issues) > 0 {
		return DeploymentNotDiscardableError{Issues: issues}
	}
	return nil
}

// contentOfProjectFile rebuilds the content of a Project's Draft from the
// ProjectFile a Deployment runs or a Commit fixed: the entities keep the
// identity they had when they were stored.
func contentOfProjectFile(projectID id.ID[project.Project], file projectv0.ProjectFile) (draft.Content, error) {
	var content draft.Content
	for _, d := range file.Devices {
		protocol, err := device.NewDeviceProtocol(d.Protocol)
		if err != nil {
			return draft.Content{}, fmt.Errorf("device %s: %w", d.ID, err)
		}
		config, err := device.NewDeviceConfig([]byte(d.Config))
		if err != nil {
			return draft.Content{}, fmt.Errorf("device %s: %w", d.ID, err)
		}
		name, err := device.NewDeviceName(d.Name)
		if err != nil {
			return draft.Content{}, fmt.Errorf("device %s: %w", d.ID, err)
		}
		created, err := device.CreateDevice(id.NewID(id.IDWithUUID[device.Device](d.ID)), projectID, name, d.Comment, protocol, config)
		if err != nil {
			return draft.Content{}, err
		}
		content.Devices = append(content.Devices, created)
	}
	for _, e := range file.Environments {
		name, err := device.NewEnvironmentName(e.Name)
		if err != nil {
			return draft.Content{}, fmt.Errorf("environment %s: %w", e.ID, err)
		}
		created, err := device.CreateEnvironment(id.NewID(id.IDWithUUID[device.Environment](e.ID)), projectID, name, e.Comment)
		if err != nil {
			return draft.Content{}, err
		}
		content.Environments = append(content.Environments, created)
	}
	for _, p := range file.EnvironmentDevices {
		params, err := device.NewEnvironmentDeviceParams([]byte(p.Params))
		if err != nil {
			return draft.Content{}, fmt.Errorf("pair %s/%s: %w", p.EnvironmentID, p.DeviceID, err)
		}
		content.EnvironmentDevices = append(content.EnvironmentDevices, device.CreateEnvironmentDevice(projectID,
			id.NewID(id.IDWithUUID[device.Environment](p.EnvironmentID)),
			id.NewID(id.IDWithUUID[device.Device](p.DeviceID)), params))
	}
	for _, t := range file.Tags {
		name, err := tag.NewTagName(t.Name)
		if err != nil {
			return draft.Content{}, err
		}
		tagType, err := tag.NewTagType(t.Type)
		if err != nil {
			return draft.Content{}, err
		}
		supplier := tag.NoSupplier()
		if t.SupplierDeviceID != nil {
			supplier = tag.SuppliedByDevice(id.NewID(id.IDWithUUID[device.Device](*t.SupplierDeviceID)))
		}
		created, err := tag.CreateTag(id.NewID(id.IDWithUUID[tag.Tag](t.ID)), projectID, name, tagType, supplier)
		if err != nil {
			return draft.Content{}, err
		}
		content.Tags = append(content.Tags, created)
	}
	for _, sc := range file.Scenes {
		built, err := sceneOfProjectFile(projectID, sc)
		if err != nil {
			return draft.Content{}, err
		}
		content.Scenes = append(content.Scenes, built)
	}
	for _, p := range file.LibraryPins {
		number, err := library.NewReleaseNumber(p.Release)
		if err != nil {
			return draft.Content{}, err
		}
		content.LibraryPins = append(content.LibraryPins, librarypin.ReconstituteLibraryPin(projectID,
			id.NewID(id.IDWithUUID[library.Library](p.LibraryID)), number))
	}
	return content, nil
}

func sceneOfProjectFile(projectID id.ID[project.Project], sc projectv0.Scene) (scene.Scene, error) {
	name, err := scene.NewSceneName(sc.Name)
	if err != nil {
		return nil, err
	}
	size, err := scene.NewSceneSize(sc.Width, sc.Height)
	if err != nil {
		return nil, err
	}
	widgets := make([]scene.Widget, 0, len(sc.Widgets))
	for _, w := range sc.Widgets {
		widgetSize, err := library.NewSize(w.Size.Width, w.Size.Height)
		if err != nil {
			return nil, err
		}
		origin, err := scene.NewOrigin(w.Origin.X, w.Origin.Y)
		if err != nil {
			return nil, err
		}
		bindings := make([]scene.PortBinding, 0, len(w.PortBindings))
		for _, b := range w.PortBindings {
			portName, err := library.NewInputPortName(b.PortName)
			if err != nil {
				return nil, err
			}
			bindings = append(bindings, scene.NewPortBinding(portName, id.NewID(id.IDWithUUID[tag.Tag](b.TagID))))
		}
		widgetName, err := scene.NewWidgetName(w.Name)
		if err != nil {
			return nil, fmt.Errorf("widget %s: %w", w.ID, err)
		}
		widgets = append(widgets, scene.NewWidget(
			id.NewID(id.IDWithUUID[scene.Widget](w.ID)), widgetName,
			scene.NewPosition(w.Position.X, w.Position.Y, w.Position.Z),
			widgetSize, origin, scene.NewRotation(w.Rotation.Degrees),
			id.NewID(id.IDWithUUID[library.WidgetType](w.TypeID)), w.Labels, bindings))
	}
	return scene.ReconstituteScene(id.NewID(id.IDWithUUID[scene.Scene](sc.ID)), projectID, name, size,
		scene.NewBackgroundHTML(sc.BackgroundHTML), widgets), nil
}
