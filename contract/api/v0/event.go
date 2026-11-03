package apiv0

// Event is the payload of every message of the event streams (sent as SSE
// `data:`), whatever the domain event. GET /events carries the events of the
// server's own resources — projects, libraries with the WidgetTypes of their
// Drafts and their releases — and of the connections,
// plus, with ?project= (EventsQueryProject), those of that project's content;
// GET /projects/{id}/events those of one project's content alone — Tags,
// Scenes, Widgets, Library Pins, Devices, Environments with the parameters of
// their Devices. Type is one of the Event… names below. ID
// is the affected aggregate's (or connection's) ID; system_ready carries
// none; the ID of a library_* or library_pin_* event is the library's.
// SceneID is the Scene that holds (or held) the Widget, on widget_created,
// widget_updated and widget_deleted only. LibraryID is the library of the
// WidgetType, on widget_type_* only. Release is the number of the new
// release, on library_released only. EnvironmentID and DeviceID are the pair
// of an environment_device_* event. Tag
// carries the Tag's full new state, in the shape of GET /tags/{id}, its seq
// included, on tag_updated only. tag_updated is delivered at-most-once: a
// reader that may have missed one rereads the tags on reconnect, and drops a
// state whose seq is not greater than the one it holds.
// Revision and DeployedAt are those of the new Revision on revision_created
// only, the signal by which a reader of a Project's running configuration
// reloads it.
// Readers skip event types they do not know: a new type is a MINOR change.
type Event struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	ID        string `json:"id,omitempty"`
	SceneID   string `json:"scene_id,omitempty"`
	LibraryID string `json:"library_id,omitempty"`
	Release   int    `json:"release,omitempty"`
	// EnvironmentID and DeviceID name the pair whose parameters changed, on
	// environment_device_* only; ID is the Environment's (the pair has no ID
	// of its own), and EnvironmentID names it again.
	EnvironmentID string       `json:"environment_id,omitempty"`
	DeviceID      string       `json:"device_id,omitempty"`
	Tag           *TagResponse `json:"tag,omitempty"`
	// Revision and DeployedAt name the Revision a revision_created created:
	// its number and the time it was deployed (RFC 3339). ID is the Project's;
	// the stream of a Project carries the event to that Project's readers.
	Revision   int    `json:"revision,omitempty"`
	DeployedAt string `json:"deployed_at,omitempty"`
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

	EventDeviceCreated            = "device_created"
	EventDeviceUpdated            = "device_updated"
	EventDeviceDeleted            = "device_deleted"
	EventEnvironmentCreated       = "environment_created"
	EventEnvironmentUpdated       = "environment_updated"
	EventEnvironmentDeleted       = "environment_deleted"
	EventEnvironmentDeviceCreated = "environment_device_created"
	EventEnvironmentDeviceUpdated = "environment_device_updated"
	EventEnvironmentDeviceDeleted = "environment_device_deleted"

	EventRevisionCreated = "revision_created"
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
		EventDeviceCreated, EventDeviceUpdated, EventDeviceDeleted,
		EventEnvironmentCreated, EventEnvironmentUpdated, EventEnvironmentDeleted,
		EventEnvironmentDeviceCreated, EventEnvironmentDeviceUpdated, EventEnvironmentDeviceDeleted,
		EventRevisionCreated,
	}
}
