package draft

import (
	"context"
	"reflect"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// Open replaces the whole content of the Draft with content, recording only
// the entities whose stored state actually differs (task 39, ADR 0018): a
// Tag, Scene, Widget, Device, Environment, pair or pin that is the same on
// both sides is neither written nor announced, so the DraftChange is
// proportional to the difference, not to two Drafts. The events are the
// ordinary events of each changed entity, created, updated or deleted, so a
// stream and the undo behave as for a hand edit (ADR 0008, ADR 0017).
func (s *Session) Open(ctx context.Context, content Content) error {
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
	s.openScenes(scenes, content.Scenes)
	s.openTags(tags, content.Tags)
	s.openDevices(devices, content.Devices)
	s.openEnvironments(environments, content.Environments)
	s.openPairs(pairs, content.EnvironmentDevices)
	s.openPins(pins, content.LibraryPins)
	return nil
}

func (s *Session) openTags(stored, wanted []tag.Tag) {
	old := make(map[uuid.UUID]tag.Tag, len(stored))
	for _, t := range stored {
		old[t.ID().UUID()] = t
	}
	seen := make(map[uuid.UUID]struct{}, len(wanted))
	for _, t := range wanted {
		id := t.ID().UUID()
		seen[id] = struct{}{}
		before, ok := old[id]
		switch {
		case !ok:
			s.recordAfter(SnapshotOfTag(t))
			s.recordEvents(tag.NewTagCreatedEvent(t.ID(), s.ProjectID()))
		case !sameSnapshotState(SnapshotOfTag(before), SnapshotOfTag(t)):
			s.recordBefore(SnapshotOfTag(before))
			s.recordAfter(SnapshotOfTag(t))
			// A Tag definition has no update of its own: changed fields are a
			// deletion and a creation, as the ordinary Tag API writes them.
			s.recordEvents(tag.NewTagDeletedEvent(before.ID(), s.ProjectID()),
				tag.NewTagCreatedEvent(t.ID(), s.ProjectID()))
		}
	}
	for _, t := range stored {
		if _, ok := seen[t.ID().UUID()]; ok {
			continue
		}
		s.recordBefore(SnapshotOfTag(t))
		s.recordEvents(tag.NewTagDeletedEvent(t.ID(), s.ProjectID()))
	}
}

func (s *Session) openDevices(stored, wanted []device.Device) {
	old := make(map[uuid.UUID]device.Device, len(stored))
	for _, d := range stored {
		old[d.ID().UUID()] = d
	}
	seen := make(map[uuid.UUID]struct{}, len(wanted))
	for _, d := range wanted {
		id := d.ID().UUID()
		seen[id] = struct{}{}
		before, ok := old[id]
		switch {
		case !ok:
			s.recordAfter(SnapshotOfDevice(d))
			s.recordEvents(device.NewDeviceCreatedEvent(d.ID(), s.ProjectID()))
		case !sameSnapshotState(SnapshotOfDevice(before), SnapshotOfDevice(d)):
			s.recordBefore(SnapshotOfDevice(before))
			s.recordAfter(SnapshotOfDevice(d))
			s.recordEvents(device.NewDeviceUpdatedEvent(d.ID(), s.ProjectID()))
		}
	}
	for _, d := range stored {
		if _, ok := seen[d.ID().UUID()]; ok {
			continue
		}
		s.recordBefore(SnapshotOfDevice(d))
		s.recordEvents(device.NewDeviceDeletedEvent(d.ID(), s.ProjectID()))
	}
}

func (s *Session) openEnvironments(stored, wanted []device.Environment) {
	old := make(map[uuid.UUID]device.Environment, len(stored))
	for _, e := range stored {
		old[e.ID().UUID()] = e
	}
	seen := make(map[uuid.UUID]struct{}, len(wanted))
	for _, e := range wanted {
		id := e.ID().UUID()
		seen[id] = struct{}{}
		before, ok := old[id]
		switch {
		case !ok:
			s.recordAfter(SnapshotOfEnvironment(e))
			s.recordEvents(device.NewEnvironmentCreatedEvent(e.ID(), s.ProjectID()))
		case !sameSnapshotState(SnapshotOfEnvironment(before), SnapshotOfEnvironment(e)):
			s.recordBefore(SnapshotOfEnvironment(before))
			s.recordAfter(SnapshotOfEnvironment(e))
			s.recordEvents(device.NewEnvironmentUpdatedEvent(e.ID(), s.ProjectID()))
		}
	}
	for _, e := range stored {
		if _, ok := seen[e.ID().UUID()]; ok {
			continue
		}
		s.recordBefore(SnapshotOfEnvironment(e))
		s.recordEvents(device.NewEnvironmentDeletedEvent(e.ID(), s.ProjectID()))
	}
}

// pairKey names a pair (Environment, Device): it has no identity of its own.
type pairKey struct {
	environment uuid.UUID
	device      uuid.UUID
}

func (s *Session) openPairs(stored, wanted []device.EnvironmentDevice) {
	old := make(map[pairKey]device.EnvironmentDevice, len(stored))
	for _, p := range stored {
		old[pairKey{p.EnvironmentID().UUID(), p.DeviceID().UUID()}] = p
	}
	seen := make(map[pairKey]struct{}, len(wanted))
	for _, p := range wanted {
		key := pairKey{p.EnvironmentID().UUID(), p.DeviceID().UUID()}
		seen[key] = struct{}{}
		before, ok := old[key]
		switch {
		case !ok:
			s.recordAfter(SnapshotOfEnvironmentDevice(p))
			s.recordEvents(device.NewEnvironmentDeviceCreatedEvent(p.EnvironmentID(), p.DeviceID(), s.ProjectID()))
		case !sameSnapshotState(SnapshotOfEnvironmentDevice(before), SnapshotOfEnvironmentDevice(p)):
			s.recordBefore(SnapshotOfEnvironmentDevice(before))
			s.recordAfter(SnapshotOfEnvironmentDevice(p))
			s.recordEvents(device.NewEnvironmentDeviceUpdatedEvent(p.EnvironmentID(), p.DeviceID(), s.ProjectID()))
		}
	}
	for _, p := range stored {
		key := pairKey{p.EnvironmentID().UUID(), p.DeviceID().UUID()}
		if _, ok := seen[key]; ok {
			continue
		}
		s.recordBefore(SnapshotOfEnvironmentDevice(p))
		s.recordEvents(device.NewEnvironmentDeviceDeletedEvent(p.EnvironmentID(), p.DeviceID(), s.ProjectID()))
	}
}

func (s *Session) openPins(stored, wanted []librarypin.LibraryPin) {
	old := make(map[uuid.UUID]librarypin.LibraryPin, len(stored))
	for _, p := range stored {
		old[p.LibraryID().UUID()] = p
	}
	seen := make(map[uuid.UUID]struct{}, len(wanted))
	for _, p := range wanted {
		id := p.LibraryID().UUID()
		seen[id] = struct{}{}
		before, ok := old[id]
		switch {
		case !ok:
			s.recordAfter(SnapshotOfLibraryPin(p))
			s.recordEvents(librarypin.NewLibraryPinCreatedEvent(p.LibraryID(), s.ProjectID()))
		case !sameSnapshotState(SnapshotOfLibraryPin(before), SnapshotOfLibraryPin(p)):
			s.recordBefore(SnapshotOfLibraryPin(before))
			s.recordAfter(SnapshotOfLibraryPin(p))
			s.recordEvents(librarypin.NewLibraryPinUpdatedEvent(p.LibraryID(), s.ProjectID()))
		}
	}
	for _, p := range stored {
		if _, ok := seen[p.LibraryID().UUID()]; ok {
			continue
		}
		s.recordBefore(SnapshotOfLibraryPin(p))
		s.recordEvents(librarypin.NewLibraryPinDeletedEvent(p.LibraryID(), s.ProjectID()))
	}
}

// placedWidget is a Widget with the Scene it is placed on: a Widget row names
// its Scene, so a Widget that moves is a removal from one and a creation on
// the other.
type placedWidget struct {
	sceneID uuid.UUID
	widget  scene.Widget
}

func (s *Session) openScenes(stored, wanted []scene.Scene) {
	oldScenes := make(map[uuid.UUID]scene.Scene, len(stored))
	oldWidgets := make(map[uuid.UUID]placedWidget)
	for _, sc := range stored {
		oldScenes[sc.ID().UUID()] = sc
		for _, w := range sc.Widgets() {
			oldWidgets[w.ID().UUID()] = placedWidget{sceneID: sc.ID().UUID(), widget: w}
		}
	}
	newScenes := make(map[uuid.UUID]scene.Scene, len(wanted))
	seenScenes := make(map[uuid.UUID]struct{}, len(wanted))
	seenWidgets := make(map[uuid.UUID]struct{})
	for _, sc := range wanted {
		sceneID := sc.ID().UUID()
		seenScenes[sceneID] = struct{}{}
		newScenes[sceneID] = sc
		for _, w := range sc.Widgets() {
			seenWidgets[w.ID().UUID()] = struct{}{}
		}
	}
	for _, sc := range wanted {
		sceneID := sc.ID().UUID()
		before, ok := oldScenes[sceneID]
		switch {
		case !ok:
			s.recordAfter(SnapshotOfScene(sc))
			s.recordEvents(scene.NewSceneCreatedEvent(sc.ID(), s.ProjectID()))
		case !sameSnapshotState(SnapshotOfScene(before), SnapshotOfScene(sc)):
			s.recordBefore(SnapshotOfScene(before))
			s.recordAfter(SnapshotOfScene(sc))
			s.recordEvents(scene.NewSceneUpdatedEvent(sc.ID(), s.ProjectID()))
		}
	}
	for _, sc := range stored {
		if _, ok := seenScenes[sc.ID().UUID()]; ok {
			continue
		}
		s.recordBefore(SnapshotOfScene(sc))
		s.recordEvents(scene.NewSceneDeletedEvent(sc.ID(), s.ProjectID()))
	}

	for _, sc := range wanted {
		sceneID := sc.ID().UUID()
		for _, w := range sc.Widgets() {
			before, ok := oldWidgets[w.ID().UUID()]
			switch {
			case !ok:
				s.recordAfter(SnapshotOfWidget(sc.ID(), w))
				s.recordEvents(scene.NewWidgetCreatedEvent(w.ID(), sc.ID(), s.ProjectID(), w.TypeID()))
			case before.sceneID != sceneID:
				s.recordBefore(SnapshotOfWidget(sceneIDOf(before.sceneID), before.widget))
				s.recordEvents(scene.NewWidgetDeletedEvent(before.widget.ID(),
					sceneIDOf(before.sceneID), s.ProjectID()))
				s.recordAfter(SnapshotOfWidget(sc.ID(), w))
				s.recordEvents(scene.NewWidgetCreatedEvent(w.ID(), sc.ID(), s.ProjectID(), w.TypeID()))
			case !sameWidget(before.widget, w):
				s.recordBefore(SnapshotOfWidget(sceneIDOf(before.sceneID), before.widget))
				s.recordAfter(SnapshotOfWidget(sc.ID(), w))
				s.recordEvents(scene.NewWidgetUpdatedEvent(w.ID(), sc.ID(), s.ProjectID(), w.TypeID()))
			}
		}
	}
	for _, sc := range stored {
		for _, w := range sc.Widgets() {
			if _, ok := seenWidgets[w.ID().UUID()]; ok {
				continue
			}
			s.recordBefore(SnapshotOfWidget(sc.ID(), w))
			s.recordEvents(scene.NewWidgetDeletedEvent(w.ID(), sc.ID(), s.ProjectID()))
		}
	}
}

// LockLibrary locks the live Library's row FOR SHARE in the Draft's
// transaction: a pin of the Library cannot be written after its deletion
// begins, the same guard a pin of the ordinary API takes (ADR 0018). It is
// the caller's check that the Release the pin names exists.
func (s *Session) LockLibrary(ctx context.Context, libraryID id.ID[library.Library]) error {
	return s.loader.LockLibrary(ctx, libraryID)
}

// sceneIDOf names a Scene by the UUID its row carries.
func sceneIDOf(raw uuid.UUID) id.ID[scene.Scene] {
	return id.NewID(id.IDWithUUID[scene.Scene](raw))
}

// sameSnapshotState reports whether two snapshots carry the same stored row:
// same kind, identity and state.
func sameSnapshotState(a, b Snapshot) bool {
	return a.Kind == b.Kind && a.ID == b.ID && reflect.DeepEqual(a.State, b.State)
}
