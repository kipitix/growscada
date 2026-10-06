package scene

import (
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// Widget - entity representing a widget instance placed on a scene.
// Widget belongs to the Scene aggregate: it has no identity of its own outside
// of the Scene that contains it, and no independent optimistic-concurrency version —
// consistency is guaranteed by the owning Scene's version.
// Each widget is an instance of a WidgetType, positioned at given coordinates,
// optionally labelled, and bound to tags via named PortBindings.
type Widget interface {
	ID() id.ID[Widget]
	Name() WidgetName
	Position() Position
	Size() library.Size
	Origin() Origin
	Rotation() Rotation
	TransformationMatrix() TransformationMatrix
	TypeID() id.ID[library.WidgetType]
	Labels() []string
	PortBindings() []PortBinding

	fmt.Stringer
}

// widgetImpl - Widget implementation struct.
type widgetImpl struct {
	id           id.ID[Widget]
	name         WidgetName
	position     Position
	size         library.Size
	origin       Origin
	rotation     Rotation
	typeID       id.ID[library.WidgetType]
	labels       []string
	portBindings []PortBinding
}

var _ Widget = (*widgetImpl)(nil)

// NewWidget creates a new Widget entity.
func NewWidget(
	anID id.ID[Widget],
	aName WidgetName,
	aPosition Position,
	aSize library.Size,
	anOrigin Origin,
	aRotation Rotation,
	aTypeID id.ID[library.WidgetType],
	someLabels []string,
	somePortBindings []PortBinding,
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
		labels:       labels,
		portBindings: portBindings,
	}
}

func (w widgetImpl) ID() id.ID[Widget]                 { return w.id }
func (w widgetImpl) Name() WidgetName                  { return w.name }
func (w widgetImpl) Position() Position                { return w.position }
func (w widgetImpl) Size() library.Size                { return w.size }
func (w widgetImpl) Origin() Origin                    { return w.origin }
func (w widgetImpl) Rotation() Rotation                { return w.rotation }
func (w widgetImpl) TypeID() id.ID[library.WidgetType] { return w.typeID }

func (w widgetImpl) Labels() []string {
	out := make([]string, len(w.labels))
	copy(out, w.labels)
	return out
}

func (w widgetImpl) PortBindings() []PortBinding {
	out := make([]PortBinding, len(w.portBindings))
	copy(out, w.portBindings)
	return out
}

// TransformationMatrix computes the 2D affine CSS matrix from the widget's
// position, origin, rotation and size.
func (w widgetImpl) TransformationMatrix() TransformationMatrix {
	return NewTransformationMatrix(w.position, w.origin, w.rotation, w.size)
}

// String implements [fmt.Stringer].
func (w widgetImpl) String() string {
	return fmt.Sprintf(
		"Widget: %s, TypeID: %s, Position: %s, Size: %s, Origin: %s, Rotation: %s",
		w.name, w.typeID, w.position, w.size, w.origin, w.rotation,
	)
}
