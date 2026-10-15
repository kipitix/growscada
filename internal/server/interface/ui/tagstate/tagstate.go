// Package tagstate keeps the state of the Tags a mode of the UI shows, fed by
// both the list loads (GET /tags) and the Tag states of tag_updated events.
// Events and loads may arrive out of order, so for each Tag the state with
// the greater Version wins.
package tagstate

import (
	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// Tags is the known state of the Tags, by ID and as a list in display order
// (the order of the last loaded list; a Tag known only from an event comes
// after). The zero value is empty. A Tags is a value: its methods return a new
// one and never modify the receiver, so a view given the old one sees no
// change under it; neither the map nor the list it returns may be modified.
type Tags struct {
	byID map[uuid.UUID]apiv0.TagResponse
	list []apiv0.TagResponse
}

// ByID returns the Tags by ID, for reading only.
func (ts Tags) ByID() map[uuid.UUID]apiv0.TagResponse { return ts.byID }

// List returns the Tags in display order, for reading only.
func (ts Tags) List() []apiv0.TagResponse { return ts.list }

// Find returns the Tag id, or false if it is not known.
func (ts Tags) Find(id uuid.UUID) (apiv0.TagResponse, bool) {
	t, ok := ts.byID[id]
	return t, ok
}

// Apply returns the Tags with t's new state recorded, unless a newer (or the
// same) Version is already known. An unknown Tag is added: the event carries
// its full state.
func (ts Tags) Apply(t apiv0.TagResponse) Tags {
	if cur, ok := ts.byID[t.ID]; ok && cur.Version >= t.Version {
		return ts
	}
	list := make([]apiv0.TagResponse, 0, len(ts.list)+1)
	replaced := false
	for _, cur := range ts.list {
		if cur.ID == t.ID {
			cur, replaced = t, true
		}
		list = append(list, cur)
	}
	if !replaced {
		list = append(list, t)
	}
	return newTags(list)
}

// Merge combines a freshly loaded Tag list with the known state: the list
// decides which Tags exist and their order, and for each Tag the state with
// the greater Version wins, so a list read before a tag_updated does not roll
// its value back.
func (ts Tags) Merge(loaded []apiv0.TagResponse) Tags {
	list := make([]apiv0.TagResponse, len(loaded))
	for i, t := range loaded {
		if cur, ok := ts.byID[t.ID]; ok && cur.Version > t.Version {
			t = cur
		}
		list[i] = t
	}
	return newTags(list)
}

// ForgetVersions returns the Tags with their states kept on screen but their
// Versions reset to 0, below any persisted Version, so the next state received
// for each Tag wins. Used on a resync: while disconnected the server may have
// been restarted with a fresh database, its Versions starting over below the
// ones known here, which would otherwise keep the stale values forever.
func (ts Tags) ForgetVersions() Tags {
	list := make([]apiv0.TagResponse, len(ts.list))
	for i, t := range ts.list {
		t.Version = 0
		list[i] = t
	}
	return newTags(list)
}

func newTags(list []apiv0.TagResponse) Tags {
	byID := make(map[uuid.UUID]apiv0.TagResponse, len(list))
	for _, t := range list {
		byID[t.ID] = t
	}
	return Tags{byID: byID, list: list}
}
