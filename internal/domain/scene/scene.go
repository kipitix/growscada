package scene

import (
	"fmt"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/version"
)

// Scene - aggregate representing a scene (a page inside a project).
// A scene has a name, canvas dimensions, a static HTML background,
// and a collection of widgets overlaid on top of it.
type Scene interface {
	ID() id.ID[Scene]
	Name() SceneName
	Size() SceneSize
	BackgroundHTML() BackgroundHTML
	Version() version.Version[Scene]

	fmt.Stringer
}

// sceneImpl - Scene implementation struct.
type sceneImpl struct {
	id             id.ID[Scene]
	name           SceneName
	size           SceneSize
	backgroundHTML BackgroundHTML
	version        version.Version[Scene]
}

var _ Scene = (*sceneImpl)(nil)

// NewScene creates a new Scene aggregate.
func NewScene(
	anID id.ID[Scene],
	aName SceneName,
	aSize SceneSize,
	aBackgroundHTML BackgroundHTML,
	aVersion version.Version[Scene],
) Scene {
	return &sceneImpl{
		id:             anID,
		name:           aName,
		size:           aSize,
		backgroundHTML: aBackgroundHTML,
		version:        aVersion,
	}
}

func (s sceneImpl) ID() id.ID[Scene]               { return s.id }
func (s sceneImpl) Name() SceneName                 { return s.name }
func (s sceneImpl) Size() SceneSize                 { return s.size }
func (s sceneImpl) BackgroundHTML() BackgroundHTML   { return s.backgroundHTML }
func (s sceneImpl) Version() version.Version[Scene] { return s.version }

// String implements [fmt.Stringer].
func (s sceneImpl) String() string {
	return fmt.Sprintf("Scene: %s, Size: %s", s.name, s.size)
}
