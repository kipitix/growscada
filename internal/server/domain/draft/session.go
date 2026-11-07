package draft

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Session is one write of a Draft, working inside the Draft's transaction
// (ADR 0016): it reads only what an operation touches through the Loader,
// applies the operation in the domain and collects the entities it changed
// (as before/after snapshots) and the events they recorded.
//
// The Draft's application calls one operation of a Session; the repository
// then writes the changed rows, the DraftChange and the outbox and commits.
// Session is the Draft's boundary in time, not a second aggregate: every
// operation below is one Engineer intent (ADR 0017).
type Session struct {
	draft  Draft
	loader Loader

	before []Snapshot
	after  []Snapshot
	events []event.Event
}

// NewSession returns the Session of the Draft over the Loader.
func NewSession(aDraft Draft, aLoader Loader) *Session {
	return &Session{draft: aDraft, loader: aLoader}
}

// Draft is the Draft the Session works on.
func (s *Session) Draft() Draft { return s.draft }

// Version is the Version of the Draft's content before the write.
func (s *Session) Version() version.Version[Draft] { return s.draft.Version() }

// Scope is the Project or Library the Draft belongs to.
func (s *Session) Scope() Scope { return s.draft.Scope() }

// ProjectID is the Project of the Draft's scope, a zero ID for a Library's.
func (s *Session) ProjectID() id.ID[project.Project] { return s.draft.Scope().ProjectID() }

// Changes returns the before and after states the Session collected, in the
// order the operations recorded them.
func (s *Session) Changes() (before, after []Snapshot) {
	return slices.Clone(s.before), slices.Clone(s.after)
}

// Events returns the events the Session's entities recorded.
func (s *Session) Events() []event.Event { return slices.Clone(s.events) }

func (s *Session) recordBefore(snapshot Snapshot) { s.before = append(s.before, snapshot) }
func (s *Session) recordAfter(snapshot Snapshot)  { s.after = append(s.after, snapshot) }

func (s *Session) recordEvents(someEvents ...event.Event) {
	s.events = append(s.events, someEvents...)
}

// Undo applies the before states of change, the DraftChange at the stack's
// position (ADR 0017): the rows the change wrote are removed, the rows it
// replaced are restored, the Draft's Version is raised and its position steps
// back. The events it records are the inverse of the change's own, so a stream
// sees the restoration as the forward edit of the same effect. An undo is not
// a DraftChange itself.
func (s *Session) Undo(_ context.Context, change DraftChange) error {
	next, err := s.draft.Undo(change)
	if err != nil {
		return err
	}
	events, err := s.restore(change.After, change.Before)
	if err != nil {
		return err
	}
	s.draft = next
	s.recordEvents(events...)
	return nil
}

// Redo applies the after states of change, the DraftChange after the stack's
// position (ADR 0017): the rows it wrote come back, the rows it replaced
// change again, and the Draft's Version is raised with its position stepped
// forward. Redo is not a DraftChange itself.
func (s *Session) Redo(_ context.Context, change DraftChange) error {
	next, err := s.draft.Redo(change)
	if err != nil {
		return err
	}
	events, err := s.restore(change.Before, change.After)
	if err != nil {
		return err
	}
	s.draft = next
	s.recordEvents(events...)
	return nil
}

// restore records the write that turns oldSide of a DraftChange into newSide
// — the snapshots are applied, the operation is never replayed — and returns
// the events it publishes.
func (s *Session) restore(oldSide, newSide []Snapshot) ([]event.Event, error) {
	for _, snapshot := range oldSide {
		s.recordBefore(snapshot)
	}
	for _, snapshot := range newSide {
		s.recordAfter(snapshot)
	}
	return inverseEvents(s.Scope(), oldSide, newSide)
}

// CreateTag records a new Tag definition.
func (s *Session) CreateTag(_ context.Context, aTag tag.Tag) tag.Tag {
	s.recordAfter(SnapshotOfTag(aTag))
	s.recordEvents(aTag.PendingEvents()...)
	return aTag
}

// DeleteTag removes a Tag definition and, in the same change, the PortBindings
// of it in every Scene of the Project (ADR 0010): what the removed policy
// used to do afterwards is now part of the intent.
func (s *Session) DeleteTag(ctx context.Context, tagID id.ID[tag.Tag]) (tag.Tag, error) {
	found, err := s.loader.Tag(ctx, tagID)
	if err != nil {
		return nil, err
	}
	deleted := found.Delete()
	s.recordBefore(SnapshotOfTag(found))
	s.recordEvents(deleted.PendingEvents()...)

	scenes, err := s.loader.Scenes(ctx)
	if err != nil {
		return nil, err
	}
	for _, sc := range scenes {
		unbound, changed := sc.UnbindTag(tagID)
		if !changed {
			continue
		}
		s.recordWidgetChanges(sc, unbound)
		s.recordEvents(unbound.PendingEvents()...)
	}
	return found, nil
}

// CreateScene records a new Scene.
func (s *Session) CreateScene(_ context.Context, aScene scene.Scene) scene.Scene {
	s.recordAfter(SnapshotOfScene(aScene))
	s.recordEvents(aScene.PendingEvents()...)
	return aScene
}

// UpdateScene changes a Scene's own fields.
func (s *Session) UpdateScene(ctx context.Context, sceneID id.ID[scene.Scene], name scene.SceneName, size scene.SceneSize, background scene.BackgroundHTML) (scene.Scene, error) {
	found, err := s.loader.Scene(ctx, sceneID)
	if err != nil {
		return nil, err
	}
	updated, err := found.Update(name, size, background)
	if err != nil {
		return nil, err
	}
	s.recordBefore(SnapshotOfScene(found))
	s.recordAfter(SnapshotOfScene(updated))
	s.recordEvents(updated.PendingEvents()...)
	return updated, nil
}

// DeleteScene removes a Scene with its Widgets: each Widget's row is recorded
// before it goes, for the undo to restore it (ADR 0017).
func (s *Session) DeleteScene(ctx context.Context, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	found, err := s.loader.Scene(ctx, sceneID)
	if err != nil {
		return nil, err
	}
	deleted := found.Delete()
	s.recordBefore(SnapshotOfScene(found))
	for _, w := range found.Widgets() {
		s.recordBefore(SnapshotOfWidget(found.ID(), w))
	}
	s.recordEvents(deleted.PendingEvents()...)
	return found, nil
}

// AddWidget places a Widget on a Scene, checking its WidgetType against the
// Library Release the Project pins and its PortBindings against the Project's
// Tags in this transaction (ADR 0012).
func (s *Session) AddWidget(ctx context.Context, sceneID id.ID[scene.Scene], w scene.Widget) (scene.Widget, error) {
	sc, err := s.loader.Scene(ctx, sceneID)
	if err != nil {
		return nil, err
	}
	wt, err := s.loader.PinnedWidgetType(ctx, w.TypeID())
	if err != nil {
		return nil, err
	}
	tagTypes, err := s.loader.TagTypes(ctx, scene.BoundTagIDs(w))
	if err != nil {
		return nil, err
	}
	changed, err := sc.AddWidget(w, wt, tagTypes)
	if err != nil {
		return nil, err
	}
	s.recordAfter(SnapshotOfWidget(sceneID, w))
	s.recordEvents(changed.PendingEvents()...)
	return w, nil
}

// UpdateWidget replaces a Widget of a Scene, checking it as AddWidget does.
func (s *Session) UpdateWidget(ctx context.Context, sceneID id.ID[scene.Scene], w scene.Widget) (scene.Widget, error) {
	sc, err := s.loader.Scene(ctx, sceneID)
	if err != nil {
		return nil, err
	}
	stored, err := sc.FindWidget(w.ID())
	if err != nil {
		return nil, err
	}
	wt, err := s.loader.PinnedWidgetType(ctx, w.TypeID())
	if err != nil {
		return nil, err
	}
	tagTypes, err := s.loader.TagTypes(ctx, scene.BoundTagIDs(w))
	if err != nil {
		return nil, err
	}
	changed, err := sc.UpdateWidget(w, wt, tagTypes)
	if err != nil {
		return nil, err
	}
	s.recordBefore(SnapshotOfWidget(sceneID, stored))
	s.recordAfter(SnapshotOfWidget(sceneID, w))
	s.recordEvents(changed.PendingEvents()...)
	return w, nil
}

// MoveWidgets moves Widgets of a Scene, recording the before and after state of
// each widget that actually moved (ADR 0017). A move changes only the
// geometry: the Scene's WidgetType and Tags are not read (ADR 0010).
func (s *Session) MoveWidgets(ctx context.Context, sceneID id.ID[scene.Scene], moves []scene.WidgetMove) ([]scene.Widget, error) {
	sc, err := s.loader.Scene(ctx, sceneID)
	if err != nil {
		return nil, err
	}
	changed, moved, err := sc.MoveWidgets(moves)
	if err != nil {
		return nil, err
	}
	for _, w := range moved {
		stored, err := sc.FindWidget(w.ID())
		if err != nil {
			return nil, err
		}
		s.recordBefore(SnapshotOfWidget(sceneID, stored))
		s.recordAfter(SnapshotOfWidget(sceneID, w))
	}
	s.recordEvents(changed.PendingEvents()...)
	return moved, nil
}

// RemoveWidget removes a Widget from its Scene.
func (s *Session) RemoveWidget(ctx context.Context, sceneID id.ID[scene.Scene], widgetID id.ID[scene.Widget]) (scene.Widget, error) {
	sc, err := s.loader.Scene(ctx, sceneID)
	if err != nil {
		return nil, err
	}
	changed, removed, err := sc.RemoveWidget(widgetID)
	if err != nil {
		return nil, err
	}
	s.recordBefore(SnapshotOfWidget(sceneID, removed))
	s.recordEvents(changed.PendingEvents()...)
	return removed, nil
}

// recordWidgetChanges records, of two states of one Scene, the Widgets that
// differ: a binding removed by a Pin switch or a Tag deletion changes Widget
// rows, never the Scene's own row.
func (s *Session) recordWidgetChanges(before, after scene.Scene) {
	stored := make(map[id.ID[scene.Widget]]scene.Widget, len(before.Widgets()))
	for _, w := range before.Widgets() {
		stored[w.ID()] = w
	}
	for _, w := range after.Widgets() {
		old, ok := stored[w.ID()]
		if ok && sameWidget(old, w) {
			continue
		}
		if ok {
			s.recordBefore(SnapshotOfWidget(before.ID(), old))
		}
		s.recordAfter(SnapshotOfWidget(after.ID(), w))
	}
}

func sameWidget(a, b scene.Widget) bool {
	return a.Name() == b.Name() &&
		a.Position() == b.Position() &&
		a.Size() == b.Size() &&
		a.Origin() == b.Origin() &&
		a.Rotation() == b.Rotation() &&
		a.TypeID() == b.TypeID() &&
		slices.Equal(a.Labels(), b.Labels()) &&
		slices.Equal(a.PortBindings(), b.PortBindings())
}

// CreateDevice records a new Device.
func (s *Session) CreateDevice(_ context.Context, d device.Device) device.Device {
	s.recordAfter(SnapshotOfDevice(d))
	s.recordEvents(d.PendingEvents()...)
	return d
}

// UpdateDevice changes a Device's fields.
func (s *Session) UpdateDevice(ctx context.Context, deviceID id.ID[device.Device], name device.DeviceName, comment string, protocol device.DeviceProtocol, config device.DeviceConfig) (device.Device, error) {
	found, err := s.loader.Device(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	updated, err := found.Update(name, comment, protocol, config)
	if err != nil {
		return nil, err
	}
	s.recordBefore(SnapshotOfDevice(found))
	s.recordAfter(SnapshotOfDevice(updated))
	s.recordEvents(updated.PendingEvents()...)
	return updated, nil
}

// DeleteDevice removes a Device no Tag is supplied by and no Environment
// holds (ADR 0008: references are refused, never cascaded).
func (s *Session) DeleteDevice(ctx context.Context, deviceID id.ID[device.Device]) (device.Device, error) {
	found, err := s.loader.Device(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	supplies, err := s.loader.TagsOfDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if supplies {
		return nil, fmt.Errorf("%w: a tag of the project is supplied by device %s", device.ErrDeviceInUse, deviceID)
	}
	pairs, err := s.loader.PairsOfDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if len(pairs) > 0 {
		return nil, fmt.Errorf("%w: device %s stands in %d environments", device.ErrDeviceInUse, deviceID, len(pairs))
	}
	deleted := found.Delete()
	s.recordBefore(SnapshotOfDevice(found))
	s.recordEvents(deleted.PendingEvents()...)
	return found, nil
}

// CreateEnvironment records a new Environment.
func (s *Session) CreateEnvironment(_ context.Context, e device.Environment) device.Environment {
	s.recordAfter(SnapshotOfEnvironment(e))
	s.recordEvents(e.PendingEvents()...)
	return e
}

// UpdateEnvironment changes an Environment's name and comment.
func (s *Session) UpdateEnvironment(ctx context.Context, environmentID id.ID[device.Environment], name device.EnvironmentName, comment string) (device.Environment, error) {
	found, err := s.loader.Environment(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	updated, err := found.Update(name, comment)
	if err != nil {
		return nil, err
	}
	s.recordBefore(SnapshotOfEnvironment(found))
	s.recordAfter(SnapshotOfEnvironment(updated))
	s.recordEvents(updated.PendingEvents()...)
	return updated, nil
}

// DeleteEnvironment removes an Environment that holds no pair.
func (s *Session) DeleteEnvironment(ctx context.Context, environmentID id.ID[device.Environment]) (device.Environment, error) {
	found, err := s.loader.Environment(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	pairs, err := s.loader.PairsOfEnvironment(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	if len(pairs) > 0 {
		return nil, fmt.Errorf("%w: environment %s holds %d devices", device.ErrEnvironmentInUse, environmentID, len(pairs))
	}
	deleted := found.Delete()
	s.recordBefore(SnapshotOfEnvironment(found))
	s.recordEvents(deleted.PendingEvents()...)
	return found, nil
}

// PutEnvironmentDevice sets the parameters of a pair: it adds the pair when
// the Environment does not hold the Device yet, else changes it. The bool
// reports whether the pair was added.
func (s *Session) PutEnvironmentDevice(ctx context.Context, environmentID id.ID[device.Environment], deviceID id.ID[device.Device], params device.EnvironmentDeviceParams) (device.EnvironmentDevice, bool, error) {
	if _, err := s.loader.Environment(ctx, environmentID); err != nil {
		return nil, false, err
	}
	if _, err := s.loader.Device(ctx, deviceID); err != nil {
		return nil, false, err
	}
	found, err := s.loader.Pair(ctx, environmentID, deviceID)
	switch {
	case errors.Is(err, device.ErrEnvironmentDeviceNotFound):
		created := device.CreateEnvironmentDevice(s.ProjectID(), environmentID, deviceID, params)
		s.recordAfter(SnapshotOfEnvironmentDevice(created))
		s.recordEvents(created.PendingEvents()...)
		return created, true, nil
	case err != nil:
		return nil, false, err
	}
	updated, err := found.UpdateParams(params)
	if err != nil {
		return nil, false, err
	}
	s.recordBefore(SnapshotOfEnvironmentDevice(found))
	s.recordAfter(SnapshotOfEnvironmentDevice(updated))
	s.recordEvents(updated.PendingEvents()...)
	return updated, false, nil
}

// DeleteEnvironmentDevice removes a pair.
func (s *Session) DeleteEnvironmentDevice(ctx context.Context, environmentID id.ID[device.Environment], deviceID id.ID[device.Device]) (device.EnvironmentDevice, error) {
	found, err := s.loader.Pair(ctx, environmentID, deviceID)
	if err != nil {
		return nil, err
	}
	deleted := found.Delete()
	s.recordBefore(SnapshotOfEnvironmentDevice(found))
	s.recordEvents(deleted.PendingEvents()...)
	return found, nil
}

// PutLibraryPin pins a Library Release in the Draft: it adds the pin when the
// Project pins no release of the Library, else switches it. A switch removes,
// in the same change, the PortBindings the new release breaks; Widgets left
// without their WidgetType refuse it (ADR 0012). The bool reports whether the
// pin was added.
func (s *Session) PutLibraryPin(ctx context.Context, libraryID id.ID[library.Library], number library.ReleaseNumber) (librarypin.LibraryPin, bool, error) {
	release, err := s.loader.Release(ctx, libraryID, number)
	if err != nil {
		return nil, false, err
	}
	found, err := s.loader.LibraryPin(ctx, libraryID)
	switch {
	case errors.Is(err, librarypin.ErrLibraryPinNotFound):
		created := librarypin.CreateLibraryPin(s.ProjectID(), release)
		s.recordAfter(SnapshotOfLibraryPin(created))
		s.recordEvents(created.PendingEvents()...)
		return created, true, nil
	case err != nil:
		return nil, false, err
	}
	from, err := s.loader.Release(ctx, libraryID, found.Release())
	if err != nil {
		return nil, false, err
	}
	if err := s.checkPinImpact(ctx, from, &release); err != nil {
		return nil, false, err
	}
	switched, err := found.Switch(release)
	if err != nil {
		return nil, false, err
	}
	s.recordBefore(SnapshotOfLibraryPin(found))
	s.recordAfter(SnapshotOfLibraryPin(switched))
	s.recordEvents(switched.PendingEvents()...)
	if err := s.reconcileScenes(ctx, &release); err != nil {
		return nil, false, err
	}
	return switched, false, nil
}

// DeleteLibraryPin removes the pin, refused while Widgets of the Project are
// of its WidgetTypes.
func (s *Session) DeleteLibraryPin(ctx context.Context, libraryID id.ID[library.Library]) (librarypin.LibraryPin, error) {
	found, err := s.loader.LibraryPin(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	from, err := s.loader.Release(ctx, libraryID, found.Release())
	if err != nil {
		return nil, err
	}
	if err := s.checkPinImpact(ctx, from, nil); err != nil {
		return nil, err
	}
	deleted := found.Delete()
	s.recordBefore(SnapshotOfLibraryPin(found))
	s.recordEvents(deleted.PendingEvents()...)
	return found, nil
}

// checkPinImpact refuses a switch or removal that would leave Widgets of the
// Project without their WidgetType (librarypin.ImpactOf).
func (s *Session) checkPinImpact(ctx context.Context, from library.LibraryRelease, to *library.LibraryRelease) error {
	scenes, err := s.loader.Scenes(ctx)
	if err != nil {
		return err
	}
	tagTypes, err := s.loader.AllTagTypes(ctx)
	if err != nil {
		return err
	}
	var toRelease library.LibraryRelease
	if to != nil {
		toRelease = *to
	}
	return librarypin.ImpactOf(placedWidgets(scenes), from, toRelease, tagTypes).Check()
}

// reconcileScenes removes the PortBindings the new release breaks from every
// Scene of the Project (ADR 0010, ADR 0012).
func (s *Session) reconcileScenes(ctx context.Context, release *library.LibraryRelease) error {
	if release == nil {
		return nil
	}
	scenes, err := s.loader.Scenes(ctx)
	if err != nil {
		return err
	}
	tagTypes, err := s.loader.AllTagTypes(ctx)
	if err != nil {
		return err
	}
	for _, sc := range scenes {
		current := sc
		seen := map[id.ID[library.WidgetType]]struct{}{}
		for _, w := range sc.Widgets() {
			if _, ok := seen[w.TypeID()]; ok {
				continue
			}
			seen[w.TypeID()] = struct{}{}
			wt, ok := (*release).FindWidgetType(w.TypeID())
			if !ok {
				continue
			}
			reconciled, changed, err := current.ReconcileWith(wt, tagTypes)
			if err != nil {
				return err
			}
			if changed {
				current = reconciled
			}
		}
		if sameSceneWidgets(sc, current) {
			continue
		}
		s.recordWidgetChanges(sc, current)
		s.recordEvents(current.PendingEvents()...)
	}
	return nil
}

// sameSceneWidgets reports whether two states of one Scene have the same
// Widgets.
func sameSceneWidgets(a, b scene.Scene) bool {
	before, after := a.Widgets(), b.Widgets()
	if len(before) != len(after) {
		return false
	}
	for i := range before {
		if !sameWidget(before[i], after[i]) {
			return false
		}
	}
	return true
}

func placedWidgets(scenes []scene.Scene) []librarypin.PlacedWidget {
	var placed []librarypin.PlacedWidget
	for _, sc := range scenes {
		for _, w := range sc.Widgets() {
			placed = append(placed, librarypin.PlacedWidget{SceneID: sc.ID(), Widget: w})
		}
	}
	return placed
}

// CreateWidgetType records a new WidgetType of the Draft's Library.
func (s *Session) CreateWidgetType(_ context.Context, wt library.WidgetType) library.WidgetType {
	s.recordAfter(SnapshotOfWidgetType(wt))
	s.recordEvents(wt.PendingEvents()...)
	return wt
}

// UpdateWidgetType changes a WidgetType of the Draft's Library: the Draft
// holds the one Version the write states, so the WidgetType itself checks
// none (ADR 0016).
func (s *Session) UpdateWidgetType(
	ctx context.Context,
	typeID id.ID[library.WidgetType],
	name library.WidgetTypeName,
	html library.HtmlTemplate,
	script library.Script,
	language library.ScriptLanguage,
	size library.Size,
	ports []library.InputPort,
) (library.WidgetType, error) {
	found, err := s.loader.WidgetType(ctx, typeID)
	if err != nil {
		return nil, err
	}
	updated, err := found.Update(name, html, script, language, size, ports)
	if err != nil {
		return nil, err
	}
	s.recordBefore(SnapshotOfWidgetType(found))
	s.recordAfter(SnapshotOfWidgetType(updated))
	s.recordEvents(updated.PendingEvents()...)
	return updated, nil
}

// DeleteWidgetType removes a WidgetType from the Draft's Library. It touches
// no Scene: Projects see only the copies the releases they pin hold (ADR 0007).
func (s *Session) DeleteWidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.WidgetType, error) {
	found, err := s.loader.WidgetType(ctx, typeID)
	if err != nil {
		return nil, err
	}
	s.recordBefore(SnapshotOfWidgetType(found))
	s.recordEvents(found.Delete().PendingEvents()...)
	return found, nil
}

// ReplaceWidgetTypes resets the Draft's Library to release (Discard, ADR 0016):
// every WidgetType the Draft holds goes, with its events, and every copy the
// release holds is recreated under its ID, with its events. The stack and the
// DraftChanges go with the caller's transaction, not here.
func (s *Session) ReplaceWidgetTypes(ctx context.Context, release library.LibraryRelease) error {
	stored, err := s.loader.WidgetTypes(ctx)
	if err != nil {
		return err
	}
	for _, wt := range stored {
		s.recordBefore(SnapshotOfWidgetType(wt))
		s.recordEvents(wt.Delete().PendingEvents()...)
	}
	for _, released := range release.WidgetTypes() {
		wt, err := library.RecreateWidgetType(released)
		if err != nil {
			return err
		}
		s.recordAfter(SnapshotOfWidgetType(wt))
		s.recordEvents(wt.PendingEvents()...)
	}
	return nil
}

// Content is the whole content of a Project's Draft as a value: what a
// Discard restores from a Deployment and what the cleanup of a deleted Project
// writes away.
type Content struct {
	Tags               []tag.Tag
	Scenes             []scene.Scene
	Devices            []device.Device
	Environments       []device.Environment
	EnvironmentDevices []device.EnvironmentDevice
	LibraryPins        []librarypin.LibraryPin
}

// Replace replaces the whole content of the Draft with content (Discard,
// ADR 0016): every entity the Draft holds goes, with its events, and every
// entity of content is created, with the identity it has in the Deployment. The
// stack and the DraftChanges go with the caller's transaction, not here.
func (s *Session) Replace(ctx context.Context, content Content) error {
	if err := s.Purge(ctx); err != nil {
		return err
	}
	for _, d := range content.Devices {
		s.CreateDevice(ctx, d)
	}
	for _, e := range content.Environments {
		s.CreateEnvironment(ctx, e)
	}
	for _, p := range content.EnvironmentDevices {
		s.recordAfter(SnapshotOfEnvironmentDevice(p))
		s.recordEvents(device.NewEnvironmentDeviceCreatedEvent(p.EnvironmentID(), p.DeviceID(), s.ProjectID()))
	}
	for _, t := range content.Tags {
		s.CreateTag(ctx, t)
	}
	for _, sc := range content.Scenes {
		s.recordAfter(SnapshotOfScene(sc))
		s.recordEvents(scene.NewSceneCreatedEvent(sc.ID(), s.ProjectID()))
		for _, w := range sc.Widgets() {
			s.recordAfter(SnapshotOfWidget(sc.ID(), w))
			s.recordEvents(scene.NewWidgetCreatedEvent(w.ID(), sc.ID(), s.ProjectID(), w.TypeID()))
		}
	}
	for _, p := range content.LibraryPins {
		s.recordAfter(SnapshotOfLibraryPin(p))
		s.recordEvents(librarypin.NewLibraryPinCreatedEvent(p.LibraryID(), s.ProjectID()))
	}
	return nil
}

// Purge removes the whole content of the Draft (the cleanup of a deleted
// Project, ADR 0006), with the events the removal records.
func (s *Session) Purge(ctx context.Context) error {
	scenes, err := s.loader.Scenes(ctx)
	if err != nil {
		return err
	}
	tags, err := s.loader.AllTags(ctx)
	if err != nil {
		return err
	}
	pairs, err := s.loader.EnvironmentDevices(ctx)
	if err != nil {
		return err
	}
	devices, err := s.loader.AllDevices(ctx)
	if err != nil {
		return err
	}
	environments, err := s.loader.AllEnvironments(ctx)
	if err != nil {
		return err
	}
	pins, err := s.loader.AllLibraryPins(ctx)
	if err != nil {
		return err
	}

	for _, sc := range scenes {
		deleted := sc.Delete()
		s.recordBefore(SnapshotOfScene(sc))
		for _, w := range sc.Widgets() {
			s.recordBefore(SnapshotOfWidget(sc.ID(), w))
		}
		s.recordEvents(deleted.PendingEvents()...)
	}
	for _, t := range tags {
		s.recordBefore(SnapshotOfTag(t))
		s.recordEvents(t.Delete().PendingEvents()...)
	}
	for _, p := range pins {
		s.recordBefore(SnapshotOfLibraryPin(p))
		s.recordEvents(p.Delete().PendingEvents()...)
	}
	for _, p := range pairs {
		s.recordBefore(SnapshotOfEnvironmentDevice(p))
		s.recordEvents(p.Delete().PendingEvents()...)
	}
	for _, d := range devices {
		s.recordBefore(SnapshotOfDevice(d))
		s.recordEvents(d.Delete().PendingEvents()...)
	}
	for _, e := range environments {
		s.recordBefore(SnapshotOfEnvironment(e))
		s.recordEvents(e.Delete().PendingEvents()...)
	}
	return nil
}
