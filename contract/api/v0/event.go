package apiv0

// Event is the payload of every message of the event stream (GET
// /events, sent as SSE `data:`), whatever the domain event. Type is one of
// the Event… names below. ID is the affected aggregate's (or connection's)
// ID; system_ready carries none. SceneID is the Scene that holds (or held)
// the Widget, on widget_created, widget_updated and widget_deleted only. Tag
// carries the Tag's full new state, in the shape of GET /tags/{id}, on
// tag_updated only.
// Readers skip event types they do not know: a new type is a MINOR change.
type Event struct {
	Type      string       `json:"type"`
	Timestamp string       `json:"timestamp"`
	ID        string       `json:"id,omitempty"`
	SceneID   string       `json:"scene_id,omitempty"`
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
	}
}
