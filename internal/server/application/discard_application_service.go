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

// DiscardService is the Engineering node's Discard (ADR 0016): it resets the
// whole Draft to the Deployment the Project runs, in one transaction over the
// Draft, and answers with the Draft's new state (task 35, Q10).
type DiscardService interface {
	// Discard resets the live Project projectID's Draft to the Deployment it
	// runs. The written Draft is one change: its content is replaced, its
	// DraftChange stack is cleared and its Version raised.
	// Returns ErrNoDeploymentToDiscard if the Project has no Deployment, and an
	// error wrapping project.ErrProjectNotFound if it is not live.
	Discard(ctx context.Context, projectID uuid.UUID) (appdto.DraftState, error)
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
// Releases (ADR 0018).
func NewDiscardService(aContent ProjectContent, aRuntime RuntimeNode,
	someLibraries library.LibraryRepository, someReleases library.LibraryReleaseRepository) DiscardService {
	return &discardServiceImpl{content: aContent, runtime: aRuntime, libraries: someLibraries, releases: someReleases}
}

func (s discardServiceImpl) Discard(ctx context.Context, rawProjectID uuid.UUID) (appdto.DraftState, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.DraftState{}, err
	}
	deployment, ok, err := s.runtime.FindCurrentDeployment(ctx, projectID.UUID())
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot read the current deployment of project %s: %w", projectID, err)
	}
	if !ok {
		return appdto.DraftState{}, fmt.Errorf("%w: %s", ErrNoDeploymentToDiscard, projectID)
	}
	content, err := contentOfDeployment(projectID, deployment.Content)
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot read the deployment of project %s: %w", projectID, err)
	}
	// A Deployment deployed from a foreign file may name Library Releases this
	// server does not have: the Draft cannot hold such pins, so the Discard is
	// refused instead of failing on the foreign key (ADR 0018).
	if err := discardableDeployment(ctx, s.libraries, s.releases, deployment.Content); err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot discard project %s: %w", projectID, err)
	}
	result, err := s.content.Drafts().Replace(ctx, s.content.ScopeOf(projectID), func(session *draft.Session) error {
		if err := s.lockDiscardedPins(ctx, content, session); err != nil {
			return err
		}
		return session.Replace(ctx, content)
	})
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot discard project %s: %w", projectID, err)
	}
	return appdto.DraftState{Version: result.Version().Number()}, nil
}

// lockDiscardedPins locks every pinned Library's row FOR SHARE in the
// Discard's transaction: a Library deleted between the check and the write
// makes the Discard refuse, not fail on the foreign key (ADR 0018).
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

// contentOfDeployment rebuilds the content of a Project's Draft from the
// ProjectFile of the Deployment it runs: the entities keep the identity they
// had when they were deployed.
func contentOfDeployment(projectID id.ID[project.Project], file projectv0.ProjectFile) (draft.Content, error) {
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
