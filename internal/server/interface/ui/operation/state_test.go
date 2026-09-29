package operation

import (
	"encoding/json"
	"testing"

	"github.com/kipitix/growscada/internal/server/interface/ui/livescene"
)

func TestReloadTargetsFor(t *testing.T) {
	cases := map[string]reloadTargets{
		"widget_type_updated": {widgetTypes: true},
		"scene_deleted":       {scenes: true},
		"widget_created":      {widgets: true},
		"tag_created":         {tags: true},
		"tag_deleted":         {tags: true},
		"tag_updated":         {}, // applied from the event itself
		"client_connected":    {},
	}
	for ev, want := range cases {
		if got := reloadTargetsFor(ev); got != want {
			t.Errorf("%s: got %+v, want %+v", ev, got, want)
		}
	}
}

func TestDecodeTagUpdate(t *testing.T) {
	raw := json.RawMessage(`{"id":"t1","name":"speed","type":"integer","value":"42","quality":"uncertain","version":3}`)
	got, ok := decodeTagUpdate(raw)
	want := livescene.Tag{ID: "t1", Name: "speed", Type: "integer", Value: "42", Quality: livescene.QualityUncertain, Version: 3}
	if !ok || got != want {
		t.Errorf("got %+v, %v; want %+v", got, ok, want)
	}
	for _, bad := range []string{``, `null`, `{}`, `not json`} {
		if _, ok := decodeTagUpdate(json.RawMessage(bad)); ok {
			t.Errorf("decodeTagUpdate(%q) should fail", bad)
		}
	}
}

func tagV(id, value string, version int) livescene.Tag {
	return livescene.Tag{ID: id, Type: "integer", Value: value, Quality: livescene.QualityGood, Version: version}
}

func TestApplyTagUpdate(t *testing.T) {
	tags := map[string]livescene.Tag{"t1": tagV("t1", "1", 5)}

	if got := applyTagUpdate(tags, tagV("t1", "2", 6)); got["t1"].Value != "2" {
		t.Errorf("a newer Version must apply, got %+v", got["t1"])
	}
	if tags["t1"].Value != "1" {
		t.Error("the given map must not be modified")
	}
	if got := applyTagUpdate(tags, tagV("t1", "0", 4)); got["t1"].Value != "1" {
		t.Errorf("an older Version must be ignored, got %+v", got["t1"])
	}
	if got := applyTagUpdate(tags, tagV("t1", "9", 5)); got["t1"].Value != "1" {
		t.Errorf("the same Version must be ignored, got %+v", got["t1"])
	}
	if got := applyTagUpdate(tags, tagV("t2", "3", 1)); got["t2"].Value != "3" || len(got) != 2 {
		t.Errorf("an unknown Tag must be added, got %+v", got)
	}
}

func TestMergeTags(t *testing.T) {
	known := map[string]livescene.Tag{
		"newer":   tagV("newer", "event", 10),
		"older":   tagV("older", "stale", 1),
		"deleted": tagV("deleted", "x", 1),
	}
	loaded := []livescene.Tag{
		tagV("newer", "list", 9),
		tagV("older", "list", 2),
		tagV("created", "list", 1),
	}

	got := mergeTags(known, loaded)

	if got["newer"].Value != "event" {
		t.Errorf("a list read before the event must not roll the value back, got %+v", got["newer"])
	}
	if got["older"].Value != "list" {
		t.Errorf("a newer list entry must win, got %+v", got["older"])
	}
	if _, ok := got["deleted"]; ok {
		t.Error("a Tag missing from the list must be dropped")
	}
	if got["created"].Value != "list" {
		t.Errorf("a new Tag from the list must be added, got %+v", got["created"])
	}
}

func TestSelectScene(t *testing.T) {
	scenes := []livescene.Scene{{ID: "a"}, {ID: "b"}}
	if got := selectScene(scenes, "b"); got != "b" {
		t.Errorf("an existing selection must be kept, got %q", got)
	}
	if got := selectScene(scenes, "gone"); got != "a" {
		t.Errorf("a deleted selection must fall back to the first Scene, got %q", got)
	}
	if got := selectScene(scenes, ""); got != "a" {
		t.Errorf("no selection must pick the first Scene, got %q", got)
	}
	if got := selectScene(nil, "a"); got != "" {
		t.Errorf("no Scenes must select nothing, got %q", got)
	}
}
