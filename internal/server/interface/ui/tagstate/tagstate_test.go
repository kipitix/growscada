package tagstate

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// id is a stable ID named for readability.
func id(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

func tagV(name, value string, version int) apiv0.TagResponse {
	return apiv0.TagResponse{ID: id(name), Name: name, Type: "integer", Value: value, Quality: "good", Version: version}
}

func names(ts Tags) []string {
	var got []string
	for _, t := range ts.List() {
		got = append(got, t.Name)
	}
	return got
}

func value(ts Tags, name string) string {
	t, _ := ts.Find(id(name))
	return t.Value
}

func TestApply(t *testing.T) {
	tags := Tags{}.Merge([]apiv0.TagResponse{tagV("t1", "1", 5), tagV("t0", "0", 1)})

	got := tags.Apply(tagV("t1", "2", 6))
	if value(got, "t1") != "2" || got.List()[0].Value != "2" {
		t.Errorf("a newer Version must apply to the map and the list, got %+v", got.List())
	}
	if value(tags, "t1") != "1" || tags.List()[0].Value != "1" {
		t.Error("the receiver must not be modified")
	}
	if got := tags.Apply(tagV("t1", "0", 4)); value(got, "t1") != "1" {
		t.Errorf("an older Version must be ignored, got %+v", got.List())
	}
	if got := tags.Apply(tagV("t1", "9", 5)); value(got, "t1") != "1" {
		t.Errorf("the same Version must be ignored, got %+v", got.List())
	}
	got = tags.Apply(tagV("t2", "3", 1))
	if value(got, "t2") != "3" || len(got.ByID()) != 3 {
		t.Errorf("an unknown Tag must be added, got %+v", got.List())
	}
	if n := names(got); len(n) != 3 || n[0] != "t1" || n[1] != "t0" || n[2] != "t2" {
		t.Errorf("an unknown Tag must come after the loaded ones, got %v", n)
	}
	if got := (Tags{}).Apply(tagV("t1", "1", 1)); value(got, "t1") != "1" {
		t.Errorf("the zero value must take a Tag, got %+v", got.List())
	}
}

func TestMerge(t *testing.T) {
	known := Tags{}.Merge([]apiv0.TagResponse{
		tagV("newer", "event", 10),
		tagV("older", "stale", 1),
		tagV("deleted", "x", 1),
	})
	loaded := []apiv0.TagResponse{
		tagV("created", "list", 1),
		tagV("older", "list", 2),
		tagV("newer", "list", 9),
	}

	got := known.Merge(loaded)

	if value(got, "newer") != "event" {
		t.Errorf("a list read before the event must not roll the value back, got %+v", got.List())
	}
	if value(got, "older") != "list" {
		t.Errorf("a newer list entry must win, got %+v", got.List())
	}
	if _, ok := got.Find(id("deleted")); ok {
		t.Error("a Tag missing from the list must be dropped")
	}
	if value(got, "created") != "list" {
		t.Errorf("a new Tag from the list must be added, got %+v", got.List())
	}
	if n := names(got); len(n) != 3 || n[0] != "created" || n[1] != "older" || n[2] != "newer" {
		t.Errorf("the list must decide the order, got %v", n)
	}
}

func TestForgetVersions_FreshStateWinsAfterVersionsStartOver(t *testing.T) {
	// Seen before the server was restarted with a fresh database.
	known := Tags{}.Merge([]apiv0.TagResponse{tagV("t1", "old", 500)})

	forgotten := known.ForgetVersions()

	if value(forgotten, "t1") != "old" {
		t.Errorf("the value must stay on screen until fresh data arrives, got %+v", forgotten.List())
	}
	if t1, _ := known.Find(id("t1")); t1.Version != 500 {
		t.Error("the receiver must not be modified")
	}
	if got := forgotten.Merge([]apiv0.TagResponse{tagV("t1", "list", 3)}); value(got, "t1") != "list" {
		t.Errorf("the reloaded list must win over a forgotten Version, got %+v", got.List())
	}
	if got := forgotten.Apply(tagV("t1", "event", 1)); value(got, "t1") != "event" {
		t.Errorf("an event must win over a forgotten Version, got %+v", got.List())
	}
}
