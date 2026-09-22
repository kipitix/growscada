package scene

import (
	"fmt"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// Scene - aggregate representing a scene (a mnemonic/synoptic display canvas).
// A scene has a name, canvas dimensions, a static HTML background,
// and a collection of widgets overlaid on top of it. Widget is an entity of
// this aggregate: it has no identity or optimistic-concurrency version of its
// own — the Scene's Version is the sole consistency boundary for both the
// scene's own fields and its widgets.
type Scene interface {
	ID() id.ID[Scene]
	Name() SceneName
	Size() SceneSize
	BackgroundHTML() BackgroundHTML
	Widgets() []widget.Widget
	Version() version.Version[Scene]

	fmt.Stringer
}

// sceneImpl - Scene implementation struct.
type sceneImpl struct {
	id             id.ID[Scene]
	name           SceneName
	size           SceneSize
	backgroundHTML BackgroundHTML
	widgets        []widget.Widget
	version        version.Version[Scene]
}

var _ Scene = (*sceneImpl)(nil)

// NewScene creates a new Scene aggregate. someWidgets may be nil when the
// caller does not need the widget collection (e.g. creating/renaming a scene).
func NewScene(
	anID id.ID[Scene],
	aName SceneName,
	aSize SceneSize,
	aBackgroundHTML BackgroundHTML,
	someWidgets []widget.Widget,
	aVersion version.Version[Scene],
) Scene {
	widgets := make([]widget.Widget, len(someWidgets))
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

func (s sceneImpl) ID() id.ID[Scene]                { return s.id }
func (s sceneImpl) Name() SceneName                 { return s.name }
func (s sceneImpl) Size() SceneSize                 { return s.size }
func (s sceneImpl) BackgroundHTML() BackgroundHTML  { return s.backgroundHTML }
func (s sceneImpl) Widgets() []widget.Widget        { return s.widgets }
func (s sceneImpl) Version() version.Version[Scene] { return s.version }

// String implements [fmt.Stringer].
func (s sceneImpl) String() string {
	return fmt.Sprintf("Scene: %s, Size: %s", s.name, s.size)
}
