package project

import (
	"net/http"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
)

func TestSceneConflictProblem_AsksToRepeatTheEdit(t *testing.T) {
	got := sceneConflictProblem(toast.Problem{
		Problem: apiv0.Problem{Title: "Conflict", Status: http.StatusConflict, Detail: "scene version conflict"},
		Method:  http.MethodPut,
	})

	if got.Status != http.StatusConflict || got.Method != http.MethodPut {
		t.Errorf("status %d, method %q; want 409, PUT", got.Status, got.Method)
	}
	if got.Title != "Scene changed" || got.Detail == "" {
		t.Errorf("toast %+v does not say that the scene changed and what to do", got)
	}
}
