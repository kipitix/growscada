package apiclient

import (
	"encoding/json"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// ParseEvent reads the data of one message of the event stream (GET
// /events) as an apiv0.Event, tolerantly (ADR 0005): fields it does not know
// are skipped, and a Tag that does not fit apiv0.TagResponse is dropped
// rather than the whole event, so the event's other fields are kept. Data
// that is not a JSON object, a known field of another shape, or an event
// without a type is no event: ok is false.
func ParseEvent(data string) (ev apiv0.Event, ok bool) {
	var msg struct {
		apiv0.Event
		// Tag hides Event.Tag: it is read on its own, below.
		Tag json.RawMessage `json:"tag"`
	}
	if err := json.Unmarshal([]byte(data), &msg); err != nil || msg.Type == "" {
		return apiv0.Event{}, false
	}
	ev = msg.Event
	if len(msg.Tag) > 0 {
		var t *apiv0.TagResponse
		if json.Unmarshal(msg.Tag, &t) == nil {
			ev.Tag = t
		}
	}
	return ev, true
}
