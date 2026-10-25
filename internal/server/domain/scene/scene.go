package scene

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Scene - aggregate representing a scene (a mnemonic/synoptic display canvas).
// A scene has a name, canvas dimensions, a static HTML background,
// and a collection of widgets overlaid on top of it. Widget is an entity of
// this aggregate: it has no identity or optimistic-concurrency version of its
// own — the Scene's Version is the sole consistency boundary for both the
// scene's own fields and its widgets. A scene belongs to exactly one Project,
// fixed at creation, and its widgets bind only Tags of that Project
// (ADR 0006).
//
// Scene is immutable: every change returns a new Scene and leaves the
// original as it was, so the original is the state "before" and the result
// the state "after". The result also records the change's events (see
// PendingEvents). A change keeps the Version; the repository raises it once
// when it saves the result.
type Scene interface {
	ID() id.ID[Scene]
	// ProjectID is the Project the scene belongs to; it never changes.
	ProjectID() id.ID[project.Project]
	Name() SceneName
	Size() SceneSize
	BackgroundHTML() BackgroundHTML
	Widgets() []Widget
	Version() version.Version[Scene]

	// FindWidget returns the scene's widget with the given ID.
	// Returns ErrWidgetNotFound if the scene has no such widget.
	FindWidget(id.ID[Widget]) (Widget, error)

	// CheckVersion reports an edit conflict: the caller changes the scene
	// having seen it at a Version other than the current one. Every change
	// that expects a Version checks it itself; CheckVersion lets a caller
	// report the conflict before it loads what the change needs.
	// Returns ErrSceneConflict if expected is not the scene's Version.
	CheckVersion(expected version.Version[Scene]) error

	// Update changes the scene's name, size and background and records a
	// SceneUpdatedEvent.
	// Returns ErrSceneConflict if expected is not the scene's Version.
	Update(expected version.Version[Scene], aName SceneName, aSize SceneSize, aBackgroundHTML BackgroundHTML) (Scene, error)

	// AddWidget places a new widget of WidgetType wt on the scene and records
	// a WidgetCreatedEvent.
	// Returns ErrSceneConflict if expected is not the scene's Version,
	// ErrWidgetAlreadyExists if the scene already holds a widget with its ID,
	// ErrWidgetTypeMismatch if wt is not the widget's type, and for a
	// PortBinding that is not valid (see checkWidgetType)
	// ErrPortNotDeclared, ErrBoundTagNotFound or ErrPortTypeMismatch.
	// tagTypes holds the TagTypes of the Tags the widget binds; a Tag that
	// is not in it as a Tag of the scene's Project does not exist.
	AddWidget(expected version.Version[Scene], w Widget, wt library.WidgetType, tagTypes TagTypes) (Scene, error)

	// UpdateWidget replaces the scene's widget with the same ID by w, of
	// WidgetType wt, and records a WidgetUpdatedEvent.
	// Returns ErrSceneConflict if expected is not the scene's Version,
	// ErrWidgetNotFound if the scene has no widget with its ID,
	// ErrWidgetTypeMismatch if wt is not the widget's type, and for a
	// PortBinding that is not valid (see checkWidgetType)
	// ErrPortNotDeclared, ErrBoundTagNotFound or ErrPortTypeMismatch.
	// Only the PortBindings the stored widget of the same type does not hold
	// are checked: one it already held and that stopped being valid since is
	// removed by policies, not a reason to refuse the change (ADR 0010).
	// tagTypes holds the TagTypes of the Tags the widget binds; a Tag that
	// is not in it as a Tag of the scene's Project does not exist.
	UpdateWidget(expected version.Version[Scene], w Widget, wt library.WidgetType, tagTypes TagTypes) (Scene, error)

	// RemoveWidget removes the widget with the given ID from the scene,
	// records a WidgetDeletedEvent and returns the widget too. Like removing
	// any aggregate, it expects no Version.
	// Returns ErrWidgetNotFound if the scene has no such widget.
	RemoveWidget(id.ID[Widget]) (Scene, Widget, error)

	// ReconcileWith removes, from the scene's widgets of WidgetType wt, the
	// PortBindings that are no longer valid: of ports wt no longer declares,
	// of Tags missing from tagTypes as Tags of the scene's Project and of Tags
	// whose TagType the port's type
	// hint no longer accepts. It records a WidgetUpdatedEvent ByReconciliation
	// for each widget it changes. The bool reports whether anything was
	// removed.
	ReconcileWith(wt library.WidgetType, tagTypes TagTypes) (Scene, bool)

	// TagsBoundByType returns the IDs of the Tags the scene's widgets of
	// WidgetType typeID bind, each once: those ReconcileWith needs in
	// tagTypes.
	TagsBoundByType(typeID id.ID[library.WidgetType]) []id.ID[tag.Tag]

	// UnbindTag removes, from the scene's widgets of every WidgetType, the
	// PortBindings of the Tag tagID, recording a WidgetUpdatedEvent
	// ByReconciliation for each widget it changes. The bool reports whether
	// anything was removed.
	UnbindTag(tagID id.ID[tag.Tag]) (Scene, bool)

	// Delete records a WidgetDeletedEvent for each of the scene's widgets,
	// then a SceneDeletedEvent; the repository's Delete removes the scene
	// with its widgets. Like removing any aggregate, it expects no Version.
	Delete() Scene

	event.Recorder
	fmt.Stringer
}

// sceneImpl - Scene implementation struct.
type sceneImpl struct {
	id             id.ID[Scene]
	projectID      id.ID[project.Project]
	name           SceneName
	size           SceneSize
	backgroundHTML BackgroundHTML
	widgets        []Widget
	version        version.Version[Scene]
	pending        []event.Event
}

var _ Scene = (*sceneImpl)(nil)

// CreateScene creates a new, not yet saved scene of the Project aProjectID
// without widgets and records a SceneCreatedEvent.
func CreateScene(anID id.ID[Scene], aProjectID id.ID[project.Project], aName SceneName, aSize SceneSize, aBackgroundHTML BackgroundHTML) Scene {
	return &sceneImpl{
		id:             anID,
		projectID:      aProjectID,
		name:           aName,
		size:           aSize,
		backgroundHTML: aBackgroundHTML,
		widgets:        []Widget{},
		version:        version.Initial[Scene](),
		pending:        []event.Event{NewSceneCreatedEvent(anID, aProjectID)},
	}
}

// ReconstituteScene rebuilds a scene as stored, at the given version; it
// records no event. For repositories. someWidgets may be nil for a scene
// without widgets.
func ReconstituteScene(
	anID id.ID[Scene],
	aProjectID id.ID[project.Project],
	aName SceneName,
	aSize SceneSize,
	aBackgroundHTML BackgroundHTML,
	someWidgets []Widget,
	aVersion version.Version[Scene],
) Scene {
	widgets := make([]Widget, len(someWidgets))
	copy(widgets, someWidgets)

	return &sceneImpl{
		id:             anID,
		projectID:      aProjectID,
		name:           aName,
		size:           aSize,
		backgroundHTML: aBackgroundHTML,
		widgets:        widgets,
		version:        aVersion,
	}
}

func (s *sceneImpl) ID() id.ID[Scene]                  { return s.id }
func (s *sceneImpl) ProjectID() id.ID[project.Project] { return s.projectID }
func (s *sceneImpl) Name() SceneName                   { return s.name }
func (s *sceneImpl) Size() SceneSize                   { return s.size }
func (s *sceneImpl) BackgroundHTML() BackgroundHTML    { return s.backgroundHTML }
func (s *sceneImpl) Version() version.Version[Scene]   { return s.version }

func (s *sceneImpl) Widgets() []Widget {
	out := make([]Widget, len(s.widgets))
	copy(out, s.widgets)
	return out
}

func (s *sceneImpl) FindWidget(widgetID id.ID[Widget]) (Widget, error) {
	i := s.widgetIndex(widgetID)
	if i < 0 {
		return nil, id.NotFound(ErrWidgetNotFound, widgetID)
	}
	return s.widgets[i], nil
}

func (s *sceneImpl) Update(expected version.Version[Scene], aName SceneName, aSize SceneSize, aBackgroundHTML BackgroundHTML) (Scene, error) {
	if err := s.CheckVersion(expected); err != nil {
		return nil, err
	}
	// The widgets are shared: no Scene ever changes its slice.
	return &sceneImpl{
		id:             s.id,
		projectID:      s.projectID,
		name:           aName,
		size:           aSize,
		backgroundHTML: aBackgroundHTML,
		widgets:        s.widgets,
		version:        s.version,
		pending:        s.record(NewSceneUpdatedEvent(s.id, s.projectID)),
	}, nil
}

func (s *sceneImpl) AddWidget(expected version.Version[Scene], w Widget, wt library.WidgetType, tagTypes TagTypes) (Scene, error) {
	if err := s.CheckVersion(expected); err != nil {
		return nil, err
	}
	if s.widgetIndex(w.ID()) >= 0 {
		return nil, fmt.Errorf("%w: %s", ErrWidgetAlreadyExists, w.ID())
	}
	if err := s.checkWidgetType(w, wt, tagTypes, nil); err != nil {
		return nil, err
	}
	return s.withWidgets(append(slices.Clip(s.widgets), w), NewWidgetCreatedEvent(w.ID(), s.id, s.projectID, w.TypeID())), nil
}

func (s *sceneImpl) UpdateWidget(expected version.Version[Scene], w Widget, wt library.WidgetType, tagTypes TagTypes) (Scene, error) {
	if err := s.CheckVersion(expected); err != nil {
		return nil, err
	}
	i := s.widgetIndex(w.ID())
	if i < 0 {
		return nil, id.NotFound(ErrWidgetNotFound, w.ID())
	}
	if err := s.checkWidgetType(w, wt, tagTypes, s.widgets[i]); err != nil {
		return nil, err
	}
	widgets := slices.Clone(s.widgets)
	widgets[i] = w
	return s.withWidgets(widgets, NewWidgetUpdatedEvent(w.ID(), s.id, s.projectID, w.TypeID())), nil
}

func (s *sceneImpl) RemoveWidget(widgetID id.ID[Widget]) (Scene, Widget, error) {
	i := s.widgetIndex(widgetID)
	if i < 0 {
		return nil, nil, id.NotFound(ErrWidgetNotFound, widgetID)
	}
	return s.withWidgets(slices.Concat(s.widgets[:i], s.widgets[i+1:]), NewWidgetDeletedEvent(widgetID, s.id, s.projectID)), s.widgets[i], nil
}

func (s *sceneImpl) ReconcileWith(wt library.WidgetType, tagTypes TagTypes) (Scene, bool) {
	ports := declaredPorts(wt)
	return s.removeBindings(func(w Widget, b PortBinding) bool {
		return w.TypeID() == wt.ID() && s.checkBinding(b, ports, tagTypes) != nil
	})
}

func (s *sceneImpl) TagsBoundByType(typeID id.ID[library.WidgetType]) []id.ID[tag.Tag] {
	var ofType []Widget
	for _, w := range s.widgets {
		if w.TypeID() == typeID {
			ofType = append(ofType, w)
		}
	}
	return BoundTagIDs(ofType...)
}

func (s *sceneImpl) UnbindTag(tagID id.ID[tag.Tag]) (Scene, bool) {
	return s.removeBindings(func(_ Widget, b PortBinding) bool {
		return b.TagID() == tagID
	})
}

// removeBindings removes from the scene's widgets the PortBindings for which
// remove reports true, recording a WidgetUpdatedEvent ByReconciliation for
// each widget it changes. The bool reports whether anything was removed.
func (s *sceneImpl) removeBindings(remove func(Widget, PortBinding) bool) (Scene, bool) {
	var widgets []Widget // cloned on the first widget that changes
	var events []event.Event
	for i, w := range s.widgets {
		bindings := w.PortBindings()
		before := len(bindings)
		kept := slices.DeleteFunc(bindings, func(b PortBinding) bool { return remove(w, b) })
		if len(kept) == before {
			continue
		}
		if widgets == nil {
			widgets = slices.Clone(s.widgets)
		}
		widgets[i] = NewWidget(
			w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
			w.TypeID(), w.Labels(), kept,
		)
		events = append(events, NewWidgetUpdatedByReconciliationEvent(w.ID(), s.id, s.projectID, w.TypeID()))
	}
	if widgets == nil {
		return s, false
	}
	return s.withWidgets(widgets, events...), true
}

func (s *sceneImpl) Delete() Scene {
	events := make([]event.Event, 0, len(s.widgets)+1)
	for _, w := range s.widgets {
		events = append(events, NewWidgetDeletedEvent(w.ID(), s.id, s.projectID))
	}
	events = append(events, NewSceneDeletedEvent(s.id, s.projectID))
	return s.withWidgets(s.widgets, events...)
}

func (s *sceneImpl) PendingEvents() []event.Event {
	return slices.Clone(s.pending)
}

func (s *sceneImpl) CheckVersion(expected version.Version[Scene]) error {
	if expected != s.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrSceneConflict, expected, s.version)
	}
	return nil
}

func (s *sceneImpl) widgetIndex(widgetID id.ID[Widget]) int {
	return slices.IndexFunc(s.widgets, func(w Widget) bool { return w.ID() == widgetID })
}

// withWidgets returns the scene with someWidgets in place of its widgets,
// having recorded someEvents. It takes someWidgets over without copying: the
// caller passes a slice of its own that nothing else changes.
func (s *sceneImpl) withWidgets(someWidgets []Widget, someEvents ...event.Event) Scene {
	return &sceneImpl{
		id:             s.id,
		projectID:      s.projectID,
		name:           s.name,
		size:           s.size,
		backgroundHTML: s.backgroundHTML,
		widgets:        someWidgets,
		version:        s.version,
		pending:        s.record(someEvents...),
	}
}

// record returns the scene's pending events followed by someEvents, in a new
// slice: the pending events of a Scene never change.
func (s *sceneImpl) record(someEvents ...event.Event) []event.Event {
	return slices.Concat(s.pending, someEvents)
}

// checkWidgetType checks that wt is the widget's type and every PortBinding
// of the widget is valid (see checkBinding), except those that stored, the
// widget as the scene holds it (nil for a new one), already held while of the
// same type.
func (s *sceneImpl) checkWidgetType(w Widget, wt library.WidgetType, tagTypes TagTypes, stored Widget) error {
	if w.TypeID() != wt.ID() {
		return fmt.Errorf("%w: widget has type %s, checked against %s", ErrWidgetTypeMismatch, w.TypeID(), wt.ID())
	}
	var held []PortBinding
	if stored != nil && stored.TypeID() == w.TypeID() {
		held = stored.PortBindings()
	}
	ports := declaredPorts(wt)
	for _, b := range w.PortBindings() {
		if slices.Contains(held, b) {
			continue
		}
		if err := s.checkBinding(b, ports, tagTypes); err != nil {
			return fmt.Errorf("%w, widget type %q", err, wt.Name().String())
		}
	}
	return nil
}

// checkBinding checks that a PortBinding is valid (ADR 0010): ports, the
// InputPorts of the widget's WidgetType by name, declare its port
// (ErrPortNotDeclared), its Tag exists in tagTypes as a Tag of the scene's
// Project (ErrBoundTagNotFound) and the port's type hint accepts the Tag's
// TagType (ErrPortTypeMismatch).
func (s *sceneImpl) checkBinding(b PortBinding, ports map[library.InputPortName]library.InputPort, tagTypes TagTypes) error {
	port, ok := ports[b.PortName()]
	if !ok {
		return fmt.Errorf("%w: port %q", ErrPortNotDeclared, b.PortName().String())
	}
	tagType, ok := tagTypes.Of(s.projectID, b.TagID())
	if !ok {
		return fmt.Errorf("%w in project %s: port %q, tag %s", ErrBoundTagNotFound, s.projectID, b.PortName().String(), b.TagID())
	}
	if !port.TypeHint().Accepts(tagType) {
		return fmt.Errorf("%w: port %q accepts %s, tag %s is %s",
			ErrPortTypeMismatch, b.PortName().String(), port.TypeHint(), b.TagID(), tagType)
	}
	return nil
}

func declaredPorts(wt library.WidgetType) map[library.InputPortName]library.InputPort {
	declared := make(map[library.InputPortName]library.InputPort, len(wt.InputPorts()))
	for _, p := range wt.InputPorts() {
		declared[p.Name()] = p
	}
	return declared
}

// String implements [fmt.Stringer].
func (s *sceneImpl) String() string {
	return fmt.Sprintf("Scene: %s, Project: %s, Size: %s", s.name, s.projectID, s.size)
}
