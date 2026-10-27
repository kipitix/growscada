package apiv0

// Event is the payload of every message of the event streams (sent as SSE
// `data:`), whatever the domain event. GET /events carries the events of the
// server's own resources — projects, libraries with the WidgetTypes of their
// Drafts and their releases — and of the connections,
// plus, with ?project= (EventsQueryProject), those of that project's content;
// GET /projects/{id}/events those of one project's content alone — Tags,
// Scenes, Widgets, Library Pins. Type is one of the Event… names below. ID
// is the affected aggregate's (or connection's) ID; system_ready carries
// none; the ID of a library_* or library_pin_* event is the library's.
// SceneID is the Scene that holds (or held) the Widget, on widget_created,
// widget_updated and widget_deleted only. LibraryID is the library of the
// WidgetType, on widget_type_* only. Release is the number of the new
// release, on library_released only. Tag
// carries the Tag's full new state, in the shape of GET /tags/{id}, its seq
// included, on tag_updated only. tag_updated is delivered at-most-once: a
// reader that may have missed one rereads the tags on reconnect, and drops a
// state whose seq is not greater than the one it holds.
// Readers skip event types they do not know: a new type is a MINOR change.
type Event struct {
	Type      string       `json:"type"`
	Timestamp string       `json:"timestamp"`
	ID        string       `json:"id,omitempty"`
	SceneID   string       `json:"scene_id,omitempty"`
	LibraryID string       `json:"library_id,omitempty"`
	Release   int          `json:"release,omitempty"`
	Tag       *TagResponse `json:"tag,omitempty"`
}

// The types of Event: the names the event stream publishes (ADR 0009).
const (
	EventSystemReady        = "system_ready"
	EventTagCreated         = "tag_created"
	EventTagUpdated         = "tag_updated"
	EventTagDeleted         = "tag_deleted"
	EventWidgetTypeCreated  = "widget_type_created"
	EventWidgetTypeUpdated  = "widget_type_updated"
	EventWidgetTypeDeleted  = "widget_type_deleted"
	EventWidgetCreated      = "widget_created"
	EventWidgetUpdated      = "widget_updated"
	EventWidgetDeleted      = "widget_deleted"
	EventSceneCreated       = "scene_created"
	EventSceneUpdated       = "scene_updated"
	EventSceneDeleted       = "scene_deleted"
	EventClientConnected    = "client_connected"
	EventClientDisconnected = "client_disconnected"
	EventProjectCreated     = "project_created"
	EventProjectUpdated     = "project_updated"
	EventProjectDeleted     = "project_deleted"

	EventLibraryCreated        = "library_created"
	EventLibraryUpdated        = "library_updated"
	EventLibraryDeleted        = "library_deleted"
	EventLibraryDiscardStarted = "library_discard_started"
	EventLibraryReleased       = "library_released"
	EventLibraryPinCreated     = "library_pin_created"
	EventLibraryPinUpdated     = "library_pin_updated"
	EventLibraryPinDeleted     = "library_pin_deleted"
)

// EventTypes returns every type of Event of this version of the contract.
func EventTypes() []string {
	return []string{
		EventSystemReady,
		EventTagCreated, EventTagUpdated, EventTagDeleted,
		EventWidgetTypeCreated, EventWidgetTypeUpdated, EventWidgetTypeDeleted,
		EventWidgetCreated, EventWidgetUpdated, EventWidgetDeleted,
		EventSceneCreated, EventSceneUpdated, EventSceneDeleted,
		EventClientConnected, EventClientDisconnected,
		EventProjectCreated, EventProjectUpdated, EventProjectDeleted,
		EventLibraryCreated, EventLibraryUpdated, EventLibraryDeleted, EventLibraryDiscardStarted, EventLibraryReleased,
		EventLibraryPinCreated, EventLibraryPinUpdated, EventLibraryPinDeleted,
	}
}
