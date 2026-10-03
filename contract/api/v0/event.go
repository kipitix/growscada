package apiv0

// Event is the payload of every message of the event stream (GET
// /events, sent as SSE `data:`), whatever the domain event. Tag carries the
// Tag's full new state, in the shape of GET /tags/{id}, on tag_updated only.
// Readers skip event types they do not know: a new type is a MINOR change.
type Event struct {
	Type      string       `json:"type"`
	Timestamp string       `json:"timestamp"`
	ID        string       `json:"id,omitempty"`
	Tag       *TagResponse `json:"tag,omitempty"`
}
