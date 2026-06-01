package widget

import "github.com/kipitix/growscada/internal/domain/id"
import "github.com/kipitix/growscada/internal/domain/tag"

// PortBinding links an InputPort (by name) to a specific Tag instance.
// Value Object.
type PortBinding struct {
	portName InputPortName
	tagID    id.ID[tag.Tag]
}

// NewPortBinding creates a PortBinding.
func NewPortBinding(portName InputPortName, tagID id.ID[tag.Tag]) PortBinding {
	return PortBinding{portName: portName, tagID: tagID}
}

func (b PortBinding) PortName() InputPortName  { return b.portName }
func (b PortBinding) TagID() id.ID[tag.Tag]    { return b.tagID }
