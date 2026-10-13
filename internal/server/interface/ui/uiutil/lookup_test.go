package uiutil

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestFindByID(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	items := []apiv0.SceneResponse{{ID: a, Name: "a"}, {ID: b, Name: "b"}}

	if got, ok := FindByID(items, b, SceneID); !ok || got.Name != "b" {
		t.Errorf("b: got %+v, %v; want the scene b", got, ok)
	}
	if _, ok := FindByID(items, uuid.New(), SceneID); ok {
		t.Error("an absent id must not be found")
	}
	items = append(items, apiv0.SceneResponse{}) // an item without an ID yet
	if i := IdxByID(items, uuid.Nil, SceneID); i != -1 {
		t.Errorf("uuid.Nil (none selected): got index %d, want -1", i)
	}
}
