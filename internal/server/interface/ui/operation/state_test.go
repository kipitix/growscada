package operation

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// id is a stable ID named for readability.
func id(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

func tagV(name, value string, version int) apiv0.TagResponse {
	return apiv0.TagResponse{ID: id(name), Type: "integer", Value: value, Quality: "good", Version: version}
}

func TestApplyTagUpdate(t *testing.T) {
	tags := map[uuid.UUID]apiv0.TagResponse{id("t1"): tagV("t1", "1", 5)}

	if got := applyTagUpdate(tags, tagV("t1", "2", 6)); got[id("t1")].Value != "2" {
		t.Errorf("a newer Version must apply, got %+v", got[id("t1")])
	}
	if tags[id("t1")].Value != "1" {
		t.Error("the given map must not be modified")
	}
	if got := applyTagUpdate(tags, tagV("t1", "0", 4)); got[id("t1")].Value != "1" {
		t.Errorf("an older Version must be ignored, got %+v", got[id("t1")])
	}
	if got := applyTagUpdate(tags, tagV("t1", "9", 5)); got[id("t1")].Value != "1" {
		t.Errorf("the same Version must be ignored, got %+v", got[id("t1")])
	}
	if got := applyTagUpdate(tags, tagV("t2", "3", 1)); got[id("t2")].Value != "3" || len(got) != 2 {
		t.Errorf("an unknown Tag must be added, got %+v", got)
	}
}

func TestMergeTags(t *testing.T) {
	known := map[uuid.UUID]apiv0.TagResponse{
		id("newer"):   tagV("newer", "event", 10),
		id("older"):   tagV("older", "stale", 1),
		id("deleted"): tagV("deleted", "x", 1),
	}
	loaded := []apiv0.TagResponse{
		tagV("newer", "list", 9),
		tagV("older", "list", 2),
		tagV("created", "list", 1),
	}

	got := mergeTags(known, loaded)

	if got[id("newer")].Value != "event" {
		t.Errorf("a list read before the event must not roll the value back, got %+v", got[id("newer")])
	}
	if got[id("older")].Value != "list" {
		t.Errorf("a newer list entry must win, got %+v", got[id("older")])
	}
	if _, ok := got[id("deleted")]; ok {
		t.Error("a Tag missing from the list must be dropped")
	}
	if got[id("created")].Value != "list" {
		t.Errorf("a new Tag from the list must be added, got %+v", got[id("created")])
	}
}

func TestForgetVersions_FreshStateWinsAfterVersionsStartOver(t *testing.T) {
	// Seen before the server was restarted with a fresh database.
	known := map[uuid.UUID]apiv0.TagResponse{id("t1"): tagV("t1", "old", 500)}

	forgotten := forgetVersions(known)

	if forgotten[id("t1")].Value != "old" {
		t.Errorf("the value must stay on screen until fresh data arrives, got %+v", forgotten[id("t1")])
	}
	if known[id("t1")].Version != 500 {
		t.Error("the given map must not be modified")
	}
	if got := mergeTags(forgotten, []apiv0.TagResponse{tagV("t1", "list", 3)}); got[id("t1")].Value != "list" {
		t.Errorf("the reloaded list must win over a forgotten Version, got %+v", got[id("t1")])
	}
	if got := applyTagUpdate(forgotten, tagV("t1", "event", 1)); got[id("t1")].Value != "event" {
		t.Errorf("an event must win over a forgotten Version, got %+v", got[id("t1")])
	}
}

func TestSelectScene(t *testing.T) {
	scenes := []apiv0.SceneResponse{{ID: id("a")}, {ID: id("b")}}
	if got := selectScene(scenes, id("b")); got != id("b") {
		t.Errorf("an existing selection must be kept, got %v", got)
	}
	if got := selectScene(scenes, id("gone")); got != id("a") {
		t.Errorf("a deleted selection must fall back to the first Scene, got %v", got)
	}
	if got := selectScene(scenes, uuid.Nil); got != id("a") {
		t.Errorf("no selection must pick the first Scene, got %v", got)
	}
	if got := selectScene(nil, id("a")); got != uuid.Nil {
		t.Errorf("no Scenes must select nothing, got %v", got)
	}
}
