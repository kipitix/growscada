package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// DiscardProjectPolicyName is the Name of the policy returned by
// NewDiscardProjectPolicy.
const DiscardProjectPolicyName = "discard_project"

// discardProjectPolicy carries out the Discard of a Project (task 35, Q10): it
// brings the Draft back to the Revision the Project runs — removing what was
// added since, restoring what was changed, recreating what was removed under
// its IDs — one aggregate per transaction, and clears the mark of
// Project.StartDiscard last; a project_discard_started event's job does it.
//
// The order is the one the Draft's references allow: the Library Pins first
// (restoring a Scene passes the check of its Widgets against them), then the
// Devices, the Environments, the pairs, the Tag definitions and the Scenes
// with their Widgets. While the mark stands no one else writes the Draft, so a
// run that failed half-way is run again from the Draft as it is: every step
// compares what is stored with the Revision before it writes.
//
// The Runtime node is left alone: the Tag states and the Device Tokens are its
// own, and a Discard changes configuration only. For a Project not being
// discarded the job does nothing.
type discardProjectPolicy struct {
	projects project.ProjectRepository
	scopes   ProjectScopes
	releases library.LibraryReleaseRepository
	runtime  RuntimeNode
}

var _ Policy = (*discardProjectPolicy)(nil)

// NewDiscardProjectPolicy returns the policy carrying out Discards of Projects;
// someReleases reads the Library Releases a restored pin names.
func NewDiscardProjectPolicy(someProjects project.ProjectRepository, someScopes ProjectScopes, someReleases library.LibraryReleaseRepository, aRuntime RuntimeNode) Policy {
	return &discardProjectPolicy{projects: someProjects, scopes: someScopes, releases: someReleases, runtime: aRuntime}
}

func (p discardProjectPolicy) Name() string { return DiscardProjectPolicyName }

// Job key: "project:<id>".
func (p discardProjectPolicy) JobKey(e event.Event) (string, bool) {
	if e.Type() != event.EventTypeProjectDiscardStarted {
		return "", false
	}
	if ev, ok := e.(project.ProjectEvent); ok {
		return projectKeyPrefix + ev.ProjectID().String(), true
	}
	return "", false
}

func (p discardProjectPolicy) Run(ctx context.Context, key string) error {
	projectID, rest, err := cutProjectKey(key)
	if err != nil {
		return err
	}
	if rest != "" {
		return fmt.Errorf("invalid job key %q", key)
	}
	found, err := p.projects.FindByID(ctx, projectID)
	if errors.Is(err, project.ErrProjectNotFound) {
		return nil // removed: its Draft went with it
	}
	if err != nil {
		return fmt.Errorf("cannot find project %s: %w", projectID, err)
	}
	if !found.IsDiscarding() {
		return nil
	}

	revision, ok, err := p.runtime.FindCurrentRevision(ctx, projectID.UUID())
	if err != nil {
		return fmt.Errorf("cannot read the current revision of project %s: %w", projectID, err)
	}
	if !ok {
		return fmt.Errorf("cannot discard project %s: it runs no revision", projectID)
	}
	file := revision.Content

	scope := p.scopes.ForDiscard(projectID)
	if err := p.restorePins(ctx, scope, file.LibraryPins); err != nil {
		return fmt.Errorf("cannot discard project %s: %w", projectID, err)
	}
	if err := restoreDevices(ctx, scope.Devices(), projectID, file.Devices); err != nil {
		return fmt.Errorf("cannot discard project %s: %w", projectID, err)
	}
	if err := restoreEnvironments(ctx, scope.Environments(), projectID, file.Environments); err != nil {
		return fmt.Errorf("cannot discard project %s: %w", projectID, err)
	}
	if err := restorePairs(ctx, scope.EnvironmentDevices(), projectID, file.EnvironmentDevices); err != nil {
		return fmt.Errorf("cannot discard project %s: %w", projectID, err)
	}
	if err := restoreTags(ctx, scope.Tags(), projectID, file.Tags); err != nil {
		return fmt.Errorf("cannot discard project %s: %w", projectID, err)
	}
	if err := p.restoreScenes(ctx, scope, projectID, file.Scenes); err != nil {
		return fmt.Errorf("cannot discard project %s: %w", projectID, err)
	}

	return retryOnRace(project.ErrProjectConflict, func() error {
		current, err := p.projects.FindByID(ctx, projectID)
		if err != nil {
			return fmt.Errorf("cannot reread project %s: %w", projectID, err)
		}
		if _, err := p.projects.Save(ctx, current.FinishDiscard()); err != nil {
			return fmt.Errorf("cannot finish the discard of project %s: %w", projectID, err)
		}
		return nil
	})
}

// restorePins brings the Project's Library Pins to the ones the document names.
// A pin is set through the aggregate's own methods, but without Impact (no
// Widget is orphaned by a Discard, by construction) and with the Version the pin
// was just read at (task 35, Q10).
func (p discardProjectPolicy) restorePins(ctx context.Context, scope ProjectScope, targets []projectv0.LibraryPin) error {
	repo := scope.LibraryPins()
	current, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list library pins: %w", err)
	}
	wanted := make(map[uuid.UUID]projectv0.LibraryPin, len(targets))
	for _, t := range targets {
		wanted[t.LibraryID] = t
	}
	byLibrary := make(map[uuid.UUID]librarypin.LibraryPin, len(current))
	for _, pin := range current {
		byLibrary[pin.LibraryID().UUID()] = pin
	}

	for _, pin := range current {
		if _, ok := wanted[pin.LibraryID().UUID()]; ok {
			continue
		}
		if err := repo.Delete(ctx, pin, nil); err != nil && !errors.Is(err, librarypin.ErrLibraryPinNotFound) {
			return fmt.Errorf("cannot remove library pin of %s: %w", pin.LibraryID(), err)
		}
		delete(byLibrary, pin.LibraryID().UUID())
	}

	for _, target := range targets {
		pin, exists := byLibrary[target.LibraryID]
		if exists && pin.Release().Int() == target.Release {
			continue
		}
		release, err := p.release(ctx, target)
		if err != nil {
			return err
		}
		changed := librarypin.CreateLibraryPin(scope.ProjectID(), release)
		if exists {
			changed, err = pin.Switch(pin.Version(), release)
			if err != nil {
				return fmt.Errorf("cannot switch the pin of library %s: %w", target.LibraryID, err)
			}
		}
		if _, err := repo.Save(ctx, changed, nil); err != nil {
			return fmt.Errorf("cannot restore the pin of library %s: %w", target.LibraryID, err)
		}
	}
	return nil
}

// release returns the Library Release the document's pin names.
func (p discardProjectPolicy) release(ctx context.Context, pin projectv0.LibraryPin) (library.LibraryRelease, error) {
	libraryID := id.NewID(id.IDWithUUID[library.Library](pin.LibraryID))
	number, err := library.NewReleaseNumber(pin.Release)
	if err != nil {
		return nil, fmt.Errorf("cannot restore the pin of library %s: %w", pin.LibraryID, err)
	}
	found, err := p.releases.FindByNumber(ctx, libraryID, number)
	if err != nil {
		return nil, fmt.Errorf("cannot read release %s of library %s: %w", number, pin.LibraryID, err)
	}
	return found, nil
}

// restoreOp is one change a Discard makes to the Draft, applied until it
// succeeds.
type restoreOp struct {
	// apply makes the change; taken reports that its name is still held by
	// another aggregate of the Draft.
	apply func(ctx context.Context) (taken bool, err error)
	// aside moves the stored aggregate the change collides with to a name no
	// other one has, so that a cycle of names can be broken.
	aside func(ctx context.Context) error
}

// applyRestores applies ops until every one is done. An op whose name is taken
// waits for the others; when a whole round waits — two Devices have swapped
// their names, say — the first one moves its stored aggregate aside, which
// frees a name, and the round runs again.
func applyRestores(ctx context.Context, ops []restoreOp) error {
	for len(ops) > 0 {
		var waiting []restoreOp
		for _, op := range ops {
			taken, err := op.apply(ctx)
			if err != nil {
				return err
			}
			if taken {
				waiting = append(waiting, op)
			}
		}
		if len(waiting) == len(ops) {
			if err := ops[0].aside(ctx); err != nil {
				return err
			}
		}
		ops = waiting
	}
	return nil
}

// asideName returns the name an aggregate of the Draft is moved to while a
// cycle of names is broken: its own, with its ID in parentheses.
func asideName(name string, anID uuid.UUID) string {
	return fmt.Sprintf("%s (%s)", name, anID)
}

// restoreDevices brings the Project's Devices to the ones the document names.
func restoreDevices(ctx context.Context, repo device.DeviceRepository, projectID id.ID[project.Project], targets []projectv0.Device) error {
	stored, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list devices: %w", err)
	}
	wanted := make(map[uuid.UUID]projectv0.Device, len(targets))
	for _, t := range targets {
		wanted[t.ID] = t
	}
	byID := make(map[uuid.UUID]device.Device, len(stored))
	for _, d := range stored {
		byID[d.ID().UUID()] = d
	}
	for _, d := range stored {
		if _, ok := wanted[d.ID().UUID()]; ok {
			continue
		}
		if err := repo.Delete(ctx, d.Delete()); err != nil && !errors.Is(err, device.ErrDeviceNotFound) {
			return fmt.Errorf("cannot remove device %s: %w", d.ID(), err)
		}
		delete(byID, d.ID().UUID())
	}

	var ops []restoreOp
	for _, target := range targets {
		existing, exists := byID[target.ID]
		if exists && sameDevice(existing, target) {
			continue
		}
		op := &deviceRestore{repo: repo, projectID: projectID, target: target, stored: existing}
		ops = append(ops, restoreOp{apply: op.apply, aside: op.aside})
	}
	return applyRestores(ctx, ops)
}

// deviceRestore is one Device a Discard restores or changes.
type deviceRestore struct {
	repo      device.DeviceRepository
	projectID id.ID[project.Project]
	target    projectv0.Device
	stored    device.Device // nil when the Draft has no such Device
}

func (r *deviceRestore) apply(ctx context.Context) (bool, error) {
	changed, err := deviceOf(r.projectID, r.target, r.stored)
	if err != nil {
		return false, err
	}
	saved, err := r.repo.Save(ctx, changed)
	if errors.Is(err, device.ErrDeviceNameTaken) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot restore device %s: %w", r.target.ID, err)
	}
	r.stored = saved
	return false, nil
}

func (r *deviceRestore) aside(ctx context.Context) error {
	if r.stored == nil {
		return fmt.Errorf("cannot break the cycle of device names at %s: the draft has no such device", r.target.ID)
	}
	current := r.stored.Name().String()
	aside := asideName(current, r.target.ID)
	if current == aside {
		return fmt.Errorf("cannot break the cycle of device names at %s", r.target.ID)
	}
	name, err := device.NewDeviceName(aside)
	if err != nil {
		return fmt.Errorf("cannot move device %s aside: %w", r.target.ID, err)
	}
	updated, err := r.stored.Update(r.stored.Version(), name, r.stored.Comment(), r.stored.Protocol(), r.stored.Config())
	if err != nil {
		return fmt.Errorf("cannot move device %s aside: %w", r.target.ID, err)
	}
	saved, err := r.repo.Save(ctx, updated)
	if err != nil {
		return fmt.Errorf("cannot move device %s aside: %w", r.target.ID, err)
	}
	r.stored = saved
	return nil
}

// deviceOf returns stored changed to the document's Device target, or a new
// Device of that ID when stored is nil.
func deviceOf(projectID id.ID[project.Project], target projectv0.Device, stored device.Device) (device.Device, error) {
	name, err := device.NewDeviceName(target.Name)
	if err != nil {
		return nil, fmt.Errorf("cannot restore device %s: %w", target.ID, err)
	}
	protocol, err := device.NewDeviceProtocol(target.Protocol)
	if err != nil {
		return nil, fmt.Errorf("cannot restore device %s: %w", target.ID, err)
	}
	config, err := device.NewDeviceConfig(target.Config)
	if err != nil {
		return nil, fmt.Errorf("cannot restore device %s: %w", target.ID, err)
	}
	deviceID := id.NewID(id.IDWithUUID[device.Device](target.ID))
	if stored == nil {
		return device.CreateDevice(deviceID, projectID, name, target.Comment, protocol, config)
	}
	return stored.Update(stored.Version(), name, target.Comment, protocol, config)
}

// sameDevice reports whether d already is the document's Device target.
func sameDevice(d device.Device, target projectv0.Device) bool {
	return d.Name().String() == target.Name &&
		d.Comment() == target.Comment &&
		d.Protocol().String() == target.Protocol &&
		sameJSONObject(d.Config().Bytes(), target.Config)
}

// restoreEnvironments brings the Project's Environments to the ones the
// document names.
func restoreEnvironments(ctx context.Context, repo device.EnvironmentRepository, projectID id.ID[project.Project], targets []projectv0.Environment) error {
	stored, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list environments: %w", err)
	}
	wanted := make(map[uuid.UUID]projectv0.Environment, len(targets))
	for _, t := range targets {
		wanted[t.ID] = t
	}
	byID := make(map[uuid.UUID]device.Environment, len(stored))
	for _, e := range stored {
		byID[e.ID().UUID()] = e
	}
	for _, e := range stored {
		if _, ok := wanted[e.ID().UUID()]; ok {
			continue
		}
		if err := repo.Delete(ctx, e.Delete()); err != nil && !errors.Is(err, device.ErrEnvironmentNotFound) {
			return fmt.Errorf("cannot remove environment %s: %w", e.ID(), err)
		}
		delete(byID, e.ID().UUID())
	}

	var ops []restoreOp
	for _, target := range targets {
		existing, exists := byID[target.ID]
		if exists && existing.Name().String() == target.Name && existing.Comment() == target.Comment {
			continue
		}
		op := &environmentRestore{repo: repo, projectID: projectID, target: target, stored: existing}
		ops = append(ops, restoreOp{apply: op.apply, aside: op.aside})
	}
	return applyRestores(ctx, ops)
}

// environmentRestore is one Environment a Discard restores or changes.
type environmentRestore struct {
	repo      device.EnvironmentRepository
	projectID id.ID[project.Project]
	target    projectv0.Environment
	stored    device.Environment // nil when the Draft has no such Environment
}

func (r *environmentRestore) apply(ctx context.Context) (bool, error) {
	name, err := device.NewEnvironmentName(r.target.Name)
	if err != nil {
		return false, fmt.Errorf("cannot restore environment %s: %w", r.target.ID, err)
	}
	var changed device.Environment
	if r.stored == nil {
		created, err := device.CreateEnvironment(id.NewID(id.IDWithUUID[device.Environment](r.target.ID)), r.projectID, name, r.target.Comment)
		if err != nil {
			return false, fmt.Errorf("cannot restore environment %s: %w", r.target.ID, err)
		}
		changed = created
	} else {
		updated, err := r.stored.Update(r.stored.Version(), name, r.target.Comment)
		if err != nil {
			return false, fmt.Errorf("cannot restore environment %s: %w", r.target.ID, err)
		}
		changed = updated
	}
	saved, err := r.repo.Save(ctx, changed)
	if errors.Is(err, device.ErrEnvironmentNameTaken) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot restore environment %s: %w", r.target.ID, err)
	}
	r.stored = saved
	return false, nil
}

func (r *environmentRestore) aside(ctx context.Context) error {
	if r.stored == nil {
		return fmt.Errorf("cannot break the cycle of environment names at %s: the draft has no such environment", r.target.ID)
	}
	current := r.stored.Name().String()
	aside := asideName(current, r.target.ID)
	if current == aside {
		return fmt.Errorf("cannot break the cycle of environment names at %s", r.target.ID)
	}
	name, err := device.NewEnvironmentName(aside)
	if err != nil {
		return fmt.Errorf("cannot move environment %s aside: %w", r.target.ID, err)
	}
	updated, err := r.stored.Update(r.stored.Version(), name, r.stored.Comment())
	if err != nil {
		return fmt.Errorf("cannot move environment %s aside: %w", r.target.ID, err)
	}
	saved, err := r.repo.Save(ctx, updated)
	if err != nil {
		return fmt.Errorf("cannot move environment %s aside: %w", r.target.ID, err)
	}
	r.stored = saved
	return nil
}

// restorePairs brings the Project's EnvironmentDevices to the ones the document
// names.
func restorePairs(ctx context.Context, repo device.EnvironmentDeviceRepository, projectID id.ID[project.Project], targets []projectv0.EnvironmentDevice) error {
	stored, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list environment devices: %w", err)
	}
	type pairKey struct{ environment, device uuid.UUID }
	wanted := make(map[pairKey]projectv0.EnvironmentDevice, len(targets))
	for _, t := range targets {
		wanted[pairKey{t.EnvironmentID, t.DeviceID}] = t
	}
	byKey := make(map[pairKey]device.EnvironmentDevice, len(stored))
	for _, pair := range stored {
		byKey[pairKey{pair.EnvironmentID().UUID(), pair.DeviceID().UUID()}] = pair
	}

	for _, pair := range stored {
		if _, ok := wanted[pairKey{pair.EnvironmentID().UUID(), pair.DeviceID().UUID()}]; ok {
			continue
		}
		if err := repo.Delete(ctx, pair.Delete()); err != nil && !errors.Is(err, device.ErrEnvironmentDeviceNotFound) {
			return fmt.Errorf("cannot remove the pair of environment %s device %s: %w", pair.EnvironmentID(), pair.DeviceID(), err)
		}
		delete(byKey, pairKey{pair.EnvironmentID().UUID(), pair.DeviceID().UUID()})
	}

	for _, target := range targets {
		params, err := device.NewEnvironmentDeviceParams(target.Params)
		if err != nil {
			return fmt.Errorf("cannot restore the pair of environment %s device %s: %w", target.EnvironmentID, target.DeviceID, err)
		}
		existing, exists := byKey[pairKey{target.EnvironmentID, target.DeviceID}]
		var changed device.EnvironmentDevice
		switch {
		case !exists:
			changed = device.CreateEnvironmentDevice(
				projectID,
				id.NewID(id.IDWithUUID[device.Environment](target.EnvironmentID)),
				id.NewID(id.IDWithUUID[device.Device](target.DeviceID)),
				params,
			)
		case sameJSONObject(existing.Params().Bytes(), target.Params):
			continue
		default:
			updated, err := existing.UpdateParams(existing.Version(), params)
			if err != nil {
				return fmt.Errorf("cannot restore the pair of environment %s device %s: %w", target.EnvironmentID, target.DeviceID, err)
			}
			changed = updated
		}
		if _, err := repo.Save(ctx, changed); err != nil {
			return fmt.Errorf("cannot restore the pair of environment %s device %s: %w", target.EnvironmentID, target.DeviceID, err)
		}
	}
	return nil
}

// restoreTags brings the Project's Tag definitions to the ones the document
// names. A Tag has no change of its own: one whose definition differs is
// removed and created again under its ID, as one added since is. Its state is
// the Runtime node's and is left alone.
func restoreTags(ctx context.Context, repo tag.TagRepository, projectID id.ID[project.Project], targets []projectv0.TagDefinition) error {
	stored, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list tags: %w", err)
	}
	kept := make(map[uuid.UUID]struct{}, len(stored))
	for _, current := range stored {
		if target, ok := tagTargetOf(targets, current.ID().UUID()); ok && sameTag(current, target) {
			kept[current.ID().UUID()] = struct{}{}
			continue
		}
		if err := repo.Delete(ctx, current.Delete()); err != nil && !errors.Is(err, tag.ErrTagNotFound) {
			return fmt.Errorf("cannot remove tag %s: %w", current.ID(), err)
		}
	}

	for _, target := range targets {
		if _, ok := kept[target.ID]; ok {
			continue
		}
		created, err := tagOf(projectID, target)
		if err != nil {
			return err
		}
		if _, err := repo.Save(ctx, created); err != nil {
			return fmt.Errorf("cannot restore tag %s: %w", target.ID, err)
		}
	}
	return nil
}

// tagTargetOf returns the document's Tag definition with the given ID.
func tagTargetOf(targets []projectv0.TagDefinition, tagID uuid.UUID) (projectv0.TagDefinition, bool) {
	for _, target := range targets {
		if target.ID == tagID {
			return target, true
		}
	}
	return projectv0.TagDefinition{}, false
}

// tagOf returns a new Tag of the document's definition.
func tagOf(projectID id.ID[project.Project], target projectv0.TagDefinition) (tag.Tag, error) {
	name, err := tag.NewTagName(target.Name)
	if err != nil {
		return nil, fmt.Errorf("cannot restore tag %s: %w", target.ID, err)
	}
	tagType, err := tag.NewTagType(target.Type)
	if err != nil {
		return nil, fmt.Errorf("cannot restore tag %s: %w", target.ID, err)
	}
	supplier := tag.NoSupplier()
	if target.SupplierDeviceID != nil {
		supplier = tag.SuppliedByDevice(id.NewID(id.IDWithUUID[device.Device](*target.SupplierDeviceID)))
	}
	created, err := tag.CreateTag(id.NewID(id.IDWithUUID[tag.Tag](target.ID)), projectID, name, tagType, supplier)
	if err != nil {
		return nil, fmt.Errorf("cannot restore tag %s: %w", target.ID, err)
	}
	return created, nil
}

// sameTag reports whether t already is the document's Tag definition target.
func sameTag(t tag.Tag, target projectv0.TagDefinition) bool {
	if t.Name().String() != target.Name || t.Type().String() != target.Type {
		return false
	}
	supplier := t.Supplier()
	if target.SupplierDeviceID == nil {
		return !supplier.HasDevice()
	}
	return supplier.HasDevice() && supplier.DeviceID().UUID() == *target.SupplierDeviceID
}

// restoreScenes brings the Project's Scenes, with their Widgets, to the ones
// the document names. A Scene is set through the aggregate's own methods, so
// its Widgets pass the check against the pinned Releases (the pins were
// restored first) and every change records its events.
func (p discardProjectPolicy) restoreScenes(ctx context.Context, scope ProjectScope, projectID id.ID[project.Project], targets []projectv0.Scene) error {
	repo := scope.Scenes()
	stored, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list scenes: %w", err)
	}
	wanted := make(map[uuid.UUID]projectv0.Scene, len(targets))
	for _, t := range targets {
		wanted[t.ID] = t
	}
	byID := make(map[uuid.UUID]scene.Scene, len(stored))
	for _, s := range stored {
		byID[s.ID().UUID()] = s
	}
	for _, s := range stored {
		if _, ok := wanted[s.ID().UUID()]; ok {
			continue
		}
		if err := repo.Delete(ctx, s.Delete()); err != nil && !errors.Is(err, scene.ErrSceneNotFound) {
			return fmt.Errorf("cannot remove scene %s: %w", s.ID(), err)
		}
		delete(byID, s.ID().UUID())
	}

	for _, target := range targets {
		current, err := p.sceneOf(ctx, scope, projectID, target, byID[target.ID])
		if err != nil {
			return err
		}
		if current == nil {
			continue // already the document's Scene
		}
		if _, err := repo.Save(ctx, current); err != nil {
			return fmt.Errorf("cannot restore scene %s: %w", target.ID, err)
		}
	}
	return nil
}

// sceneOf returns stored brought to the document's Scene target, or nil when
// it already is. stored is nil when the Draft has no such Scene.
func (p discardProjectPolicy) sceneOf(ctx context.Context, scope ProjectScope, projectID id.ID[project.Project], target projectv0.Scene, stored scene.Scene) (scene.Scene, error) {
	name, err := scene.NewSceneName(target.Name)
	if err != nil {
		return nil, fmt.Errorf("cannot restore scene %s: %w", target.ID, err)
	}
	size, err := scene.NewSceneSize(target.Width, target.Height)
	if err != nil {
		return nil, fmt.Errorf("cannot restore scene %s: %w", target.ID, err)
	}
	background := scene.NewBackgroundHTML(target.BackgroundHTML)
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](target.ID))

	changed := false
	current := stored
	if stored == nil {
		current = scene.CreateScene(sceneID, projectID, name, size, background)
		changed = true
	} else if current.Name() != name || current.Size() != size || current.BackgroundHTML() != background {
		current, err = current.Update(current.Version(), name, size, background)
		if err != nil {
			return nil, fmt.Errorf("cannot restore scene %s: %w", target.ID, err)
		}
		changed = true
	}

	targets := make(map[uuid.UUID]projectv0.Widget, len(target.Widgets))
	for _, w := range target.Widgets {
		targets[w.ID] = w
	}
	for _, w := range current.Widgets() {
		if _, ok := targets[w.ID().UUID()]; ok {
			continue
		}
		current, _, err = current.RemoveWidget(w.ID())
		if err != nil {
			return nil, fmt.Errorf("cannot restore scene %s: %w", target.ID, err)
		}
		changed = true
	}

	tagTypes, err := p.tagTypesOf(ctx, scope, projectID, target.Widgets)
	if err != nil {
		return nil, err
	}
	pinned := make(map[uuid.UUID]library.ReleasedWidgetType)
	for _, targetWidget := range target.Widgets {
		w, err := widgetOf(targetWidget)
		if err != nil {
			return nil, err
		}
		existing, exists := findSceneWidget(current, targetWidget.ID)
		if exists && sameWidget(existing, targetWidget) {
			continue
		}
		wt, ok := pinned[targetWidget.TypeID]
		if !ok {
			wt, err = scope.LibraryPins().FindPinnedWidgetType(ctx, w.TypeID())
			if err != nil {
				return nil, fmt.Errorf("cannot restore widget %s of scene %s: %w", targetWidget.ID, target.ID, err)
			}
			pinned[targetWidget.TypeID] = wt
		}
		if exists {
			current, err = current.UpdateWidget(current.Version(), w, wt, tagTypes)
		} else {
			current, err = current.AddWidget(current.Version(), w, wt, tagTypes)
		}
		if err != nil {
			return nil, fmt.Errorf("cannot restore widget %s of scene %s: %w", targetWidget.ID, target.ID, err)
		}
		changed = true
	}

	if !changed {
		return nil, nil
	}
	return current, nil
}

// tagTypesOf reads the TagTypes the Widgets of a Scene bind, for the Scene's
// changes to check their PortBindings.
func (p discardProjectPolicy) tagTypesOf(ctx context.Context, scope ProjectScope, projectID id.ID[project.Project], widgets []projectv0.Widget) (scene.TagTypes, error) {
	var tagIDs []id.ID[tag.Tag]
	seen := make(map[id.ID[tag.Tag]]struct{})
	for _, w := range widgets {
		for _, b := range w.PortBindings {
			tagID := id.NewID(id.IDWithUUID[tag.Tag](b.TagID))
			if _, ok := seen[tagID]; ok {
				continue
			}
			seen[tagID] = struct{}{}
			tagIDs = append(tagIDs, tagID)
		}
	}
	types, err := scope.Tags().FindTypesByIDs(ctx, tagIDs)
	if err != nil {
		return scene.TagTypes{}, fmt.Errorf("cannot read the tag types of project %s: %w", projectID, err)
	}
	return scene.NewTagTypes(projectID, types), nil
}

// widgetOf returns the document's Widget as the Scene aggregate takes it.
func widgetOf(target projectv0.Widget) (scene.Widget, error) {
	name, err := scene.NewWidgetName(target.Name)
	if err != nil {
		return nil, fmt.Errorf("cannot restore widget %s: %w", target.ID, err)
	}
	size, err := library.NewSize(target.Size.Width, target.Size.Height)
	if err != nil {
		return nil, fmt.Errorf("cannot restore widget %s: %w", target.ID, err)
	}
	origin, err := scene.NewOrigin(target.Origin.X, target.Origin.Y)
	if err != nil {
		return nil, fmt.Errorf("cannot restore widget %s: %w", target.ID, err)
	}
	bindings := make([]scene.PortBinding, len(target.PortBindings))
	for i, b := range target.PortBindings {
		portName, err := library.NewInputPortNameFromStorage(b.PortName)
		if err != nil {
			return nil, fmt.Errorf("cannot restore widget %s: %w", target.ID, err)
		}
		bindings[i] = scene.NewPortBinding(portName, id.NewID(id.IDWithUUID[tag.Tag](b.TagID)))
	}
	return scene.NewWidget(
		id.NewID(id.IDWithUUID[scene.Widget](target.ID)),
		name,
		scene.NewPosition(target.Position.X, target.Position.Y, target.Position.Z),
		size,
		origin,
		scene.NewRotation(target.Rotation.Degrees),
		id.NewID(id.IDWithUUID[library.WidgetType](target.TypeID)),
		target.Labels,
		bindings,
	), nil
}

// findSceneWidget returns the Scene's Widget with the given ID.
func findSceneWidget(s scene.Scene, widgetID uuid.UUID) (scene.Widget, bool) {
	for _, w := range s.Widgets() {
		if w.ID().UUID() == widgetID {
			return w, true
		}
	}
	return nil, false
}

// sameWidget reports whether w already is the document's Widget target.
func sameWidget(w scene.Widget, target projectv0.Widget) bool {
	if w.Name().String() != target.Name ||
		w.Position() != scene.NewPosition(target.Position.X, target.Position.Y, target.Position.Z) ||
		w.TypeID().UUID() != target.TypeID ||
		w.Rotation() != scene.NewRotation(target.Rotation.Degrees) ||
		!slices.Equal(w.Labels(), target.Labels) {
		return false
	}
	size, err := library.NewSize(target.Size.Width, target.Size.Height)
	if err != nil || w.Size() != size {
		return false
	}
	origin, err := scene.NewOrigin(target.Origin.X, target.Origin.Y)
	if err != nil || w.Origin() != origin {
		return false
	}
	bindings := w.PortBindings()
	if len(bindings) != len(target.PortBindings) {
		return false
	}
	for i, b := range bindings {
		if b.PortName().String() != target.PortBindings[i].PortName || b.TagID().UUID() != target.PortBindings[i].TagID {
			return false
		}
	}
	return true
}

// sameJSONObject reports whether raw is obj, both canonicalized: their keys
// may be in any order.
func sameJSONObject(raw []byte, obj projectv0.JSONObject) bool {
	return bytes.Equal(canonicalJSONObject(projectv0.JSONObject(raw)), canonicalJSONObject(obj))
}
