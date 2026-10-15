package operation

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// id is a stable ID named for readability.
func id(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

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
