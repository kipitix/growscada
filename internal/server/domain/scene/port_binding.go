package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// PortBinding links an InputPort (by name) to a specific Tag instance.
// Value Object.
type PortBinding struct {
	portName library.InputPortName
	tagID    id.ID[tag.Tag]
}

// NewPortBinding creates a PortBinding.
func NewPortBinding(portName library.InputPortName, tagID id.ID[tag.Tag]) PortBinding {
	return PortBinding{portName: portName, tagID: tagID}
}

func (b PortBinding) PortName() library.InputPortName { return b.portName }
func (b PortBinding) TagID() id.ID[tag.Tag]           { return b.tagID }
