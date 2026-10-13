package operation

import (
	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// applyTagUpdate returns the Tags with t's new state recorded, unless a newer
// (or the same) Version is already known — events and list loads may arrive
// out of order. An unknown Tag is added: the event carries its full state.
// The given map is not modified: a new one lets the view notice the change.
func applyTagUpdate(tags map[uuid.UUID]apiv0.TagResponse, t apiv0.TagResponse) map[uuid.UUID]apiv0.TagResponse {
	if cur, ok := tags[t.ID]; ok && cur.Version >= t.Version {
		return tags
	}
	updated := make(map[uuid.UUID]apiv0.TagResponse, len(tags)+1)
	for id, cur := range tags {
		updated[id] = cur
	}
	updated[t.ID] = t
	return updated
}

// mergeTags combines a freshly loaded Tag list with the known state: the list
// decides which Tags exist, and for each Tag the state with the higher
// Version wins, so a list read before a tag_updated does not roll its value
// back.
func mergeTags(known map[uuid.UUID]apiv0.TagResponse, loaded []apiv0.TagResponse) map[uuid.UUID]apiv0.TagResponse {
	merged := make(map[uuid.UUID]apiv0.TagResponse, len(loaded))
	for _, t := range loaded {
		if cur, ok := known[t.ID]; ok && cur.Version > t.Version {
			t = cur
		}
		merged[t.ID] = t
	}
	return merged
}

// forgetVersions returns the Tags with their states kept on screen but their
// Versions reset to 0, below any persisted Version, so the next state received
// for each Tag wins. Used on a resync: while disconnected the server may have
// been restarted with a fresh database, its Versions starting over below the
// ones known here, which would otherwise keep the stale values forever.
func forgetVersions(tags map[uuid.UUID]apiv0.TagResponse) map[uuid.UUID]apiv0.TagResponse {
	forgotten := make(map[uuid.UUID]apiv0.TagResponse, len(tags))
	for id, t := range tags {
		t.Version = 0
		forgotten[id] = t
	}
	return forgotten
}

// selectScene keeps the selected Scene if it still exists, otherwise falls
// back to the first Scene (uuid.Nil when there are none).
func selectScene(scenes []apiv0.SceneResponse, selected uuid.UUID) uuid.UUID {
	for _, sc := range scenes {
		if sc.ID == selected {
			return selected
		}
	}
	if len(scenes) > 0 {
		return scenes[0].ID
	}
	return uuid.Nil
}
