package operation

import (
	"encoding/json"

	"github.com/kipitix/growscada/internal/server/interface/ui/livescene"
)

// reloadTargets says which server-backed lists a server event makes stale.
type reloadTargets struct {
	widgetTypes, scenes, widgets, tags bool
}

// reloadTargetsFor routes a server event. tag_updated carries the Tag's new
// state and is applied directly (applyTagUpdate), so it reloads nothing.
func reloadTargetsFor(eventType string) reloadTargets {
	switch eventType {
	case "widget_type_created", "widget_type_updated", "widget_type_deleted":
		return reloadTargets{widgetTypes: true}
	case "scene_created", "scene_updated", "scene_deleted":
		return reloadTargets{scenes: true}
	case "widget_created", "widget_updated", "widget_deleted":
		return reloadTargets{widgets: true}
	case "tag_created", "tag_deleted":
		return reloadTargets{tags: true}
	}
	return reloadTargets{}
}

// allTargets is every list: what a reconnection reloads, since the events
// published while disconnected are lost.
var allTargets = reloadTargets{widgetTypes: true, scenes: true, widgets: true, tags: true}

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
