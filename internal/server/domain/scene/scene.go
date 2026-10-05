package scene

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Scene - aggregate representing a scene (a mnemonic/synoptic display canvas).
// A scene has a name, canvas dimensions, a static HTML background,
// and a collection of widgets overlaid on top of it. Widget is an entity of
// this aggregate: it has no identity or optimistic-concurrency version of its
// own — the Scene's Version is the sole consistency boundary for both the
// scene's own fields and its widgets.
//
// Scene is immutable: every change returns a new Scene and leaves the
// original as it was, so the original is the state "before" and the result
// the state "after". A change keeps the Version; the repository raises it
// once when it saves the result.
type Scene interface {
	ID() id.ID[Scene]
	Name() SceneName
	Size() SceneSize
	BackgroundHTML() BackgroundHTML
	Widgets() []Widget
	Version() version.Version[Scene]

	// FindWidget returns the scene's widget with the given ID.
	// Returns ErrWidgetNotFound if the scene has no such widget.
	FindWidget(id.ID[Widget]) (Widget, error)

	// Update changes the scene's name, size and background.
	// Returns ErrSceneConflict if expected is not the scene's Version.
	Update(expected version.Version[Scene], aName SceneName, aSize SceneSize, aBackgroundHTML BackgroundHTML) (Scene, error)

	// AddWidget places a new widget of WidgetType wt on the scene.
	// Returns ErrSceneConflict if expected is not the scene's Version,
	// ErrWidgetAlreadyExists if the scene already holds a widget with its ID,
	// ErrWidgetTypeMismatch if wt is not the widget's type and
	// ErrPortNotDeclared if the widget binds a port wt does not declare.
	AddWidget(expected version.Version[Scene], w Widget, wt library.WidgetType) (Scene, error)

	// UpdateWidget replaces the scene's widget with the same ID by w, of
	// WidgetType wt.
	// Returns ErrSceneConflict if expected is not the scene's Version,
	// ErrWidgetNotFound if the scene has no widget with its ID,
	// ErrWidgetTypeMismatch if wt is not the widget's type and
	// ErrPortNotDeclared if the widget binds a port wt does not declare.
	UpdateWidget(expected version.Version[Scene], w Widget, wt library.WidgetType) (Scene, error)

	// RemoveWidget removes the widget with the given ID from the scene. Like
	// removing any aggregate, it expects no Version.
	// Returns ErrWidgetNotFound if the scene has no such widget.
	RemoveWidget(id.ID[Widget]) (Scene, error)

	// ReconcileWith removes, from the scene's widgets of WidgetType wt, the
	// PortBindings of ports wt no longer declares. The bool reports whether
	// anything was removed.
	ReconcileWith(wt library.WidgetType) (Scene, bool)

	fmt.Stringer
}

// sceneImpl - Scene implementation struct.
type sceneImpl struct {
	id             id.ID[Scene]
	name           SceneName
	size           SceneSize
	backgroundHTML BackgroundHTML
	widgets        []Widget
	version        version.Version[Scene]
}

var _ Scene = (*sceneImpl)(nil)

// NewScene creates a new Scene aggregate. someWidgets may be nil for a scene
// without widgets.
func NewScene(
	anID id.ID[Scene],
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
		name:           aName,
		size:           aSize,
		backgroundHTML: aBackgroundHTML,
		widgets:        widgets,
		version:        aVersion,
	}
}

func (s *sceneImpl) ID() id.ID[Scene]                { return s.id }
func (s *sceneImpl) Name() SceneName                 { return s.name }
func (s *sceneImpl) Size() SceneSize                 { return s.size }
func (s *sceneImpl) BackgroundHTML() BackgroundHTML  { return s.backgroundHTML }
func (s *sceneImpl) Version() version.Version[Scene] { return s.version }

func (s *sceneImpl) Widgets() []Widget {
	out := make([]Widget, len(s.widgets))
	copy(out, s.widgets)
	return out
}

func (s *sceneImpl) FindWidget(widgetID id.ID[Widget]) (Widget, error) {
	i := s.widgetIndex(widgetID)
	if i < 0 {
		return nil, ErrWidgetNotFound
	}
	return s.widgets[i], nil
}

func (s *sceneImpl) Update(expected version.Version[Scene], aName SceneName, aSize SceneSize, aBackgroundHTML BackgroundHTML) (Scene, error) {
	if err := s.checkVersion(expected); err != nil {
		return nil, err
	}
	return NewScene(s.id, aName, aSize, aBackgroundHTML, s.widgets, s.version), nil
}

func (s *sceneImpl) AddWidget(expected version.Version[Scene], w Widget, wt library.WidgetType) (Scene, error) {
	if err := s.checkVersion(expected); err != nil {
		return nil, err
	}
	if s.widgetIndex(w.ID()) >= 0 {
		return nil, fmt.Errorf("%w: %s", ErrWidgetAlreadyExists, w.ID())
	}
	if err := checkWidgetType(w, wt); err != nil {
		return nil, err
	}
	return s.withWidgets(append(s.Widgets(), w)), nil
}

func (s *sceneImpl) UpdateWidget(expected version.Version[Scene], w Widget, wt library.WidgetType) (Scene, error) {
	if err := s.checkVersion(expected); err != nil {
		return nil, err
	}
	i := s.widgetIndex(w.ID())
	if i < 0 {
		return nil, ErrWidgetNotFound
	}
	if err := checkWidgetType(w, wt); err != nil {
		return nil, err
	}
	widgets := s.Widgets()
	widgets[i] = w
	return s.withWidgets(widgets), nil
}

func (s *sceneImpl) RemoveWidget(widgetID id.ID[Widget]) (Scene, error) {
	i := s.widgetIndex(widgetID)
	if i < 0 {
		return nil, ErrWidgetNotFound
	}
	return s.withWidgets(slices.Delete(s.Widgets(), i, i+1)), nil
}

func (s *sceneImpl) ReconcileWith(wt library.WidgetType) (Scene, bool) {
	declared := declaredPorts(wt)
	widgets := s.Widgets()
	changed := false
	for i, w := range widgets {
		if w.TypeID() != wt.ID() {
			continue
		}
		kept := slices.DeleteFunc(w.PortBindings(), func(b PortBinding) bool {
			_, ok := declared[b.PortName()]
			return !ok
		})
		if len(kept) == len(w.PortBindings()) {
			continue
		}
		widgets[i] = NewWidget(
			w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
			w.TypeID(), w.Labels(), kept,
		)
		changed = true
	}
	if !changed {
		return s, false
	}
	return s.withWidgets(widgets), true
}

// checkVersion reports an edit conflict: the caller changes a scene it saw
// at a Version other than the current one.
func (s *sceneImpl) checkVersion(expected version.Version[Scene]) error {
	if expected != s.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrSceneConflict, expected, s.version)
	}
	return nil
}

func (s *sceneImpl) widgetIndex(widgetID id.ID[Widget]) int {
	return slices.IndexFunc(s.widgets, func(w Widget) bool { return w.ID() == widgetID })
}

func (s *sceneImpl) withWidgets(someWidgets []Widget) Scene {
	return NewScene(s.id, s.name, s.size, s.backgroundHTML, someWidgets, s.version)
}

// checkWidgetType checks that wt is the widget's type and declares every
// port the widget binds.
func checkWidgetType(w Widget, wt library.WidgetType) error {
	if w.TypeID() != wt.ID() {
		return fmt.Errorf("%w: widget has type %s, checked against %s", ErrWidgetTypeMismatch, w.TypeID(), wt.ID())
	}
	declared := declaredPorts(wt)
	for _, b := range w.PortBindings() {
		if _, ok := declared[b.PortName()]; !ok {
			return fmt.Errorf("%w: port %q, widget type %q", ErrPortNotDeclared, b.PortName().String(), wt.Name().String())
		}
	}
	return nil
}

func declaredPorts(wt library.WidgetType) map[library.InputPortName]struct{} {
	declared := make(map[library.InputPortName]struct{}, len(wt.InputPorts()))
	for _, p := range wt.InputPorts() {
		declared[p.Name()] = struct{}{}
	}
	return declared
}

// String implements [fmt.Stringer].
func (s *sceneImpl) String() string {
	return fmt.Sprintf("Scene: %s, Size: %s", s.name, s.size)
}
