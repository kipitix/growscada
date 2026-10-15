// Package tagstate keeps the state of the Tags a mode of the UI shows, fed by
// both the list loads (GET /tags) and the Tag states of tag_updated events.
// Events and loads may arrive out of order, so for each Tag the state with
// the greater Version wins. Which Tags exist is decided by the lists alone:
// an event of a Tag the last list did not hold (created since, or deleted
// before it) does not show it.
package tagstate

import (
	"maps"
	"slices"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// Tags is the known state of the Tags, as a list in display order (that of
// the last loaded list), found by ID. The zero value is empty. A Tags is a
// value: its methods return a new one and never modify the receiver, so a
// view given the old one sees no change under it; the list it returns may not
// be modified.
type Tags struct {
	list []apiv0.TagResponse
	// idx is the position of each Tag in list. Only a list changes it, so a
	// new state of a Tag shares it and copies the list alone.
	idx map[uuid.UUID]int
	// unlisted holds the states events carried of Tags the list does not
	// hold, for the next list to merge if it holds them.
	unlisted map[uuid.UUID]apiv0.TagResponse
	// epoch counts the ForgetVersions.
	epoch uint64
}

// Mark is the moment a list was requested, as Tags.Mark took it.
type Mark uint64

// Mark returns the moment of a list requested now; take it before sending.
func (ts Tags) Mark() Mark { return Mark(ts.epoch) }

// List returns the Tags in display order, for reading only.
func (ts Tags) List() []apiv0.TagResponse { return ts.list }

// Find returns the Tag id, or false if it is not known.
func (ts Tags) Find(id uuid.UUID) (apiv0.TagResponse, bool) {
	i, ok := ts.idx[id]
	if !ok {
		return apiv0.TagResponse{}, false
	}
	return ts.list[i], true
}

// Apply returns the Tags with t's new state recorded, unless a newer (or the
// same) Version is already known. A Tag the list does not hold stays unshown:
// its state is kept for the next list, which decides whether it exists (a
// list read before the event was committed would else lose it).
func (ts Tags) Apply(t apiv0.TagResponse) Tags {
	i, listed := ts.idx[t.ID]
	if !listed {
		cur, listed := ts.unlisted[t.ID]
		if listed && cur.Version >= t.Version {
			return ts
		}
		unlisted := maps.Clone(ts.unlisted)
		if unlisted == nil {
			unlisted = make(map[uuid.UUID]apiv0.TagResponse, 1)
		}
		unlisted[t.ID] = t
		ts.unlisted = unlisted
		return ts
	}
	if ts.list[i].Version >= t.Version {
		return ts
	}
	ts.list = slices.Clone(ts.list)
	ts.list[i] = t
	return ts
}

// Merge combines a freshly loaded Tag list with the known state: the list
// decides which Tags exist and their order, and for each Tag the state with
// the greater Version wins, so a list read before a tag_updated does not roll
// its value back. m is the moment the list was requested: a list requested
// before ForgetVersions is dropped, its Versions may be the server's old ones
// (the reread the forget goes with brings the list).
func (ts Tags) Merge(m Mark, loaded []apiv0.TagResponse) Tags {
	if uint64(m) < ts.epoch {
		return ts
	}
	list := make([]apiv0.TagResponse, len(loaded))
	for i, t := range loaded {
		if cur, ok := ts.Find(t.ID); ok && cur.Version > t.Version {
			t = cur
		}
		if cur, ok := ts.unlisted[t.ID]; ok && cur.Version > t.Version {
			t = cur
		}
		list[i] = t
	}
	merged := ts.with(list)
	merged.unlisted = nil
	return merged
}

// ForgetVersions returns the Tags with their states kept on screen but their
// Versions reset to 0, below any persisted Version, so the next state received
// for each Tag wins. Used on a resync: while disconnected the server may have
// been restarted with a fresh database, its Versions starting over below the
// ones known here, which would otherwise keep the stale values forever. A
// list requested before is dropped by Merge.
func (ts Tags) ForgetVersions() Tags {
	list := make([]apiv0.TagResponse, len(ts.list))
	for i, t := range ts.list {
		t.Version = 0
		list[i] = t
	}
	ts.list = list // the same Tags at the same positions: idx stays
	ts.unlisted = nil
	ts.epoch++
	return ts
}

// with returns the Tags holding list, with ts's unlisted states and epoch.
func (ts Tags) with(list []apiv0.TagResponse) Tags {
	idx := make(map[uuid.UUID]int, len(list))
	for i, t := range list {
		idx[t.ID] = i
	}
	return Tags{list: list, idx: idx, unlisted: ts.unlisted, epoch: ts.epoch}
}
