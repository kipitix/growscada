package widget

import (
	"fmt"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/version"
)

// Widget - aggregate representing a widget instance placed on a scene.
// Each widget is an instance of a WidgetType, positioned at given coordinates,
// optionally labelled, and bound to tags via named PortBindings.
type Widget interface {
	ID() id.ID[Widget]
	Name() WidgetName
	Position() Position
	Size() Size
	Origin() Origin
	Rotation() Rotation
	TransformationMatrix() TransformationMatrix
	TypeID() id.ID[WidgetType]
	SceneID() id.ID[scene.Scene]
	Labels() []string
	PortBindings() []PortBinding
	Version() version.Version[Widget]

	fmt.Stringer
}

// widgetImpl - Widget implementation struct.
type widgetImpl struct {
	id           id.ID[Widget]
	name         WidgetName
	position     Position
	size         Size
	origin       Origin
	rotation     Rotation
	typeID       id.ID[WidgetType]
	sceneID      id.ID[scene.Scene]
	labels       []string
	portBindings []PortBinding
	version      version.Version[Widget]
}

var _ Widget = (*widgetImpl)(nil)

// NewWidget creates a new Widget aggregate.
func NewWidget(
	anID id.ID[Widget],
	aName WidgetName,
	aPosition Position,
	aSize Size,
	anOrigin Origin,
	aRotation Rotation,
	aTypeID id.ID[WidgetType],
	aSceneID id.ID[scene.Scene],
	someLabels []string,
	somePortBindings []PortBinding,
	aVersion version.Version[Widget],
) Widget {
	labels := make([]string, len(someLabels))
	copy(labels, someLabels)

	portBindings := make([]PortBinding, len(somePortBindings))
	copy(portBindings, somePortBindings)

	return &widgetImpl{
		id:           anID,
		name:         aName,
		position:     aPosition,
		size:         aSize,
		origin:       anOrigin,
		rotation:     aRotation,
		typeID:       aTypeID,
		sceneID:      aSceneID,
		labels:       labels,
		portBindings: portBindings,
		version:      aVersion,
	}
}

func (w widgetImpl) ID() id.ID[Widget]               { return w.id }
func (w widgetImpl) Name() WidgetName                 { return w.name }
func (w widgetImpl) Position() Position               { return w.position }
func (w widgetImpl) Size() Size                       { return w.size }
func (w widgetImpl) Origin() Origin                   { return w.origin }
func (w widgetImpl) Rotation() Rotation               { return w.rotation }
func (w widgetImpl) TypeID() id.ID[WidgetType]        { return w.typeID }
func (w widgetImpl) SceneID() id.ID[scene.Scene]      { return w.sceneID }
func (w widgetImpl) Labels() []string                 { return w.labels }
func (w widgetImpl) PortBindings() []PortBinding      { return w.portBindings }
func (w widgetImpl) Version() version.Version[Widget] { return w.version }

// TransformationMatrix computes the 2D affine CSS matrix from the widget's
// position, origin, rotation and size.
func (w widgetImpl) TransformationMatrix() TransformationMatrix {
	return NewTransformationMatrix(w.position, w.origin, w.rotation, w.size)
}

// String implements [fmt.Stringer].
func (w widgetImpl) String() string {
	return fmt.Sprintf(
		"Widget: %s, TypeID: %s, SceneID: %s, Position: %s, Size: %s, Origin: %s, Rotation: %s",
		w.name, w.typeID, w.sceneID, w.position, w.size, w.origin, w.rotation,
	)
}
