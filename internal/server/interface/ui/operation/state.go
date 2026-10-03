package operation

import (
	"encoding/json"

	"github.com/kipitix/growscada/internal/server/interface/ui/livescene"
)

// decodeTagUpdate extracts the Tag state carried by a tag_updated event.
func decodeTagUpdate(raw json.RawMessage) (livescene.Tag, bool) {
	if len(raw) == 0 {
		return livescene.Tag{}, false
	}
	var t livescene.Tag
	if err := json.Unmarshal(raw, &t); err != nil || t.ID == "" {
		return livescene.Tag{}, false
	}
	return t, true
}

// applyTagUpdate returns the Tags with t's new state recorded, unless a newer
// (or the same) Version is already known — events and list loads may arrive
// out of order. An unknown Tag is added: the event carries its full state.
// The given map is not modified: a new one lets the view notice the change.
func applyTagUpdate(tags map[string]livescene.Tag, t livescene.Tag) map[string]livescene.Tag {
	if cur, ok := tags[t.ID]; ok && cur.Version >= t.Version {
		return tags
	}
	updated := make(map[string]livescene.Tag, len(tags)+1)
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
func mergeTags(known map[string]livescene.Tag, loaded []livescene.Tag) map[string]livescene.Tag {
	merged := make(map[string]livescene.Tag, len(loaded))
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
func forgetVersions(tags map[string]livescene.Tag) map[string]livescene.Tag {
	forgotten := make(map[string]livescene.Tag, len(tags))
	for id, t := range tags {
		t.Version = 0
		forgotten[id] = t
	}
	return forgotten
}

// selectScene keeps the selected Scene if it still exists, otherwise falls
// back to the first Scene ("" when there are none).
func selectScene(scenes []livescene.Scene, selected string) string {
	for _, sc := range scenes {
		if sc.ID == selected {
			return selected
		}
	}
	if len(scenes) > 0 {
		return scenes[0].ID
	}
	return ""
}
