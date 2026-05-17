package widget

import (
	"fmt"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
)

// Widget - aggregate representing a widget instance placed on a scene.
// Each widget is an instance of a WidgetType, positioned at given coordinates,
// optionally labelled, and bound to a set of data tags.
type Widget interface {
	ID() id.ID[Widget]
	Name() WidgetName
	Coordinates() Coordinates
	TypeID() id.ID[WidgetType]
	SceneID() id.ID[scene.Scene]
	Labels() []string
	TagIDs() []id.ID[tag.Tag]
	Version() version.Version[Widget]

	fmt.Stringer
}

// widgetImpl - Widget implementation struct.
type widgetImpl struct {
	id          id.ID[Widget]
	name        WidgetName
	coordinates Coordinates
	typeID      id.ID[WidgetType]
	sceneID     id.ID[scene.Scene]
	labels      []string
	tagIDs      []id.ID[tag.Tag]
	version     version.Version[Widget]
}

var _ Widget = (*widgetImpl)(nil)

// NewWidget creates a new Widget aggregate.
func NewWidget(
	anID id.ID[Widget],
	aName WidgetName,
	aCoordinates Coordinates,
	aTypeID id.ID[WidgetType],
	aSceneID id.ID[scene.Scene],
	someLabels []string,
	someTagIDs []id.ID[tag.Tag],
	aVersion version.Version[Widget],
) (Widget, error) {
	labels := make([]string, len(someLabels))
	copy(labels, someLabels)

	tagIDs := make([]id.ID[tag.Tag], len(someTagIDs))
	copy(tagIDs, someTagIDs)

	return &widgetImpl{
		id:          anID,
		name:        aName,
		coordinates: aCoordinates,
		typeID:      aTypeID,
		sceneID:     aSceneID,
		labels:      labels,
		tagIDs:      tagIDs,
		version:     aVersion,
	}, nil
}

func (w widgetImpl) ID() id.ID[Widget]               { return w.id }
func (w widgetImpl) Name() WidgetName                 { return w.name }
func (w widgetImpl) Coordinates() Coordinates         { return w.coordinates }
func (w widgetImpl) TypeID() id.ID[WidgetType]        { return w.typeID }
func (w widgetImpl) SceneID() id.ID[scene.Scene]      { return w.sceneID }
func (w widgetImpl) Labels() []string                 { return w.labels }
func (w widgetImpl) TagIDs() []id.ID[tag.Tag]         { return w.tagIDs }
func (w widgetImpl) Version() version.Version[Widget] { return w.version }

// String implements [fmt.Stringer].
func (w widgetImpl) String() string {
	return fmt.Sprintf("Widget: %s, TypeID: %s, SceneID: %s, Coords: %s", w.name, w.typeID, w.sceneID, w.coordinates)
}
