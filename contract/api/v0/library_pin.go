package apiv0

import (
	"github.com/google/uuid"
)

// LibraryPinImpactQueryRelease is the parameter of GET
// /projects/{id}/library-pins/{library_id}/impact naming the release to
// preview pinning; without it the preview is of removing the pin.
const LibraryPinImpactQueryRelease = "release"

// LibraryPinResponse is the HTTP DTO of a Library Pin: the Library Release,
// by its number, the project uses of the library.
type LibraryPinResponse struct {
	LibraryID uuid.UUID `json:"library_id"`
	Release   int       `json:"release"`
	Version   int       `json:"version"`
}

// GetLibraryPinsResponse is the HTTP DTO for the pins of a project, in the
// order they were added.
type GetLibraryPinsResponse struct {
	LibraryPins []LibraryPinResponse `json:"library_pins"`
}

// PutLibraryPinRequest is the HTTP DTO of PUT
// /projects/{id}/library-pins/{library_id}: pin the release numbered
// Release. Version 0 adds the pin (the library must not be pinned yet);
// any other switches it and must match the pin's current version.
type PutLibraryPinRequest struct {
	Release int `json:"release"`
	Version int `json:"version"`
}

// PinnedWidgetResponse names a Widget of the project: its Scene, its name
// and the WidgetType it is of.
type PinnedWidgetResponse struct {
	SceneID  uuid.UUID `json:"scene_id"`
	WidgetID uuid.UUID `json:"widget_id"`
	Name     string    `json:"name"`
	TypeID   uuid.UUID `json:"type_id"`
}

// BrokenBindingResponse is a PortBinding a switch of a pin would remove: its
// port is no longer declared, or its type hint no longer accepts the Tag.
type BrokenBindingResponse struct {
	PinnedWidgetResponse
	PortName string    `json:"port_name"`
	TagID    uuid.UUID `json:"tag_id"`
}

// PinImpactResponse is the HTTP DTO of the preview of switching or removing
// a pin. OrphanedWidgets would be left without their WidgetType: while there
// are any, the switch or removal is refused (409). BrokenBindings would be
// removed after it.
type PinImpactResponse struct {
	OrphanedWidgets []PinnedWidgetResponse  `json:"orphaned_widgets"`
	BrokenBindings  []BrokenBindingResponse `json:"broken_bindings"`
}
