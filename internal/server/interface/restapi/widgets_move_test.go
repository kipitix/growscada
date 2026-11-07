package restapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// The group move route: PATCH …/scenes/{scene_id}/widgets (task 40, part 1).

func TestPatchWidgets_Valid_Returns200WithNewVersion(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	first := createWidgetViaService(t, sc.ID, sc.Version, testWidgetInput)
	secondInput := testWidgetInput
	secondInput.Name = "second-gauge"
	second := createWidgetViaService(t, sc.ID, testDraftVersion(t, mainProject), secondInput)
	router := newRouterWithWidgets()

	rec := callAs(t, router, http.MethodPatch, movePath(sc.ID), apiv0.MoveWidgetsRequest{
		Version: testDraftVersion(t, mainProject),
		Moves: []apiv0.WidgetMoveRequest{
			{WidgetID: first.ID, X: 100, Y: 200},
			{WidgetID: second.ID, X: 300, Y: 400},
		},
	}, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("GrowSCADA-Schema-Version"); got == "" {
		t.Error("the schema version header is missing")
	}
	var resp apiv0.MoveWidgetsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Version == 0 {
		t.Error("the answer carries no Version")
	}

	listed, err := newServices().scenes.FindWidgetsBySceneID(context.Background(), mainProject, sc.ID)
	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	for _, tt := range []struct {
		id   uuid.UUID
		x, y float64
	}{{first.ID, 100, 200}, {second.ID, 300, 400}} {
		found := false
		for _, w := range listed {
			if w.ID != tt.id {
				continue
			}
			found = true
			if w.X != tt.x || w.Y != tt.y {
				t.Errorf("widget %s: (%v, %v), want (%v, %v)", tt.id, w.X, w.Y, tt.x, tt.y)
			}
		}
		if !found {
			t.Errorf("widget %s is not in the scene", tt.id)
		}
	}
}

func TestPatchWidgets_EmptyMoves_Returns400(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)

	rec := callAs(t, newRouterWithWidgets(), http.MethodPatch, movePath(sc.ID), apiv0.MoveWidgetsRequest{
		Version: testDraftVersion(t, mainProject),
	}, "")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400: %s", rec.Code, rec.Body)
	}
}

func TestPatchWidgets_RepeatedWidget_Returns400(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	created := createWidgetViaService(t, sc.ID, sc.Version, testWidgetInput)

	rec := callAs(t, newRouterWithWidgets(), http.MethodPatch, movePath(sc.ID), apiv0.MoveWidgetsRequest{
		Version: testDraftVersion(t, mainProject),
		Moves: []apiv0.WidgetMoveRequest{
			{WidgetID: created.ID, X: 1, Y: 2},
			{WidgetID: created.ID, X: 3, Y: 4},
		},
	}, "")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400: %s", rec.Code, rec.Body)
	}
}

func TestPatchWidgets_UnknownField_Returns400(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)

	body := json.RawMessage(`{"version":` + strconv.Itoa(testDraftVersion(t, mainProject)) +
		`,"moves":[{"widget_id":"` + uuid.New().String() + `","x":1,"y":2}],"extra":true}`)
	rec := callAs(t, newRouterWithWidgets(), http.MethodPatch, movePath(sc.ID), body, "")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400: %s", rec.Code, rec.Body)
	}
}

// A widget named by the body, not the path, is a 404 naming it: the route has
// no widget_id path parameter to read.
func TestPatchWidgets_WidgetNotInScene_Returns404(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	missing := uuid.New()

	rec := callAs(t, newRouterWithWidgets(), http.MethodPatch, movePath(sc.ID), apiv0.MoveWidgetsRequest{
		Version: testDraftVersion(t, mainProject),
		Moves:   []apiv0.WidgetMoveRequest{{WidgetID: missing, X: 1, Y: 2}},
	}, "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404: %s", rec.Code, rec.Body)
	}
	var problem apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(problem.Detail, missing.String()) {
		t.Errorf("the 404 names %q, want the missing widget %s", problem.Detail, missing)
	}
}

func TestPatchWidgets_StaleVersion_Returns409(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	created := createWidgetViaService(t, sc.ID, sc.Version, testWidgetInput)

	rec := callAs(t, newRouterWithWidgets(), http.MethodPatch, movePath(sc.ID), apiv0.MoveWidgetsRequest{
		Version: testDraftVersion(t, mainProject) + 99,
		Moves:   []apiv0.WidgetMoveRequest{{WidgetID: created.ID, X: 1, Y: 2}},
	}, "")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", rec.Code, rec.Body)
	}
	var problem apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if problem.Draft == nil {
		t.Error("the 409 carries no draft state")
	}
}

func TestPatchWidgets_NothingToMove_Returns409(t *testing.T) {
	cleanWidgetsRest(t)
	sc := createSceneViaSceneService(t)
	created := createWidgetViaService(t, sc.ID, sc.Version, testWidgetInput)
	router := newRouterWithWidgets()

	rec := callAs(t, router, http.MethodPatch, movePath(sc.ID), apiv0.MoveWidgetsRequest{
		Version: testDraftVersion(t, mainProject),
		Moves:   []apiv0.WidgetMoveRequest{{WidgetID: created.ID, X: 50, Y: 60}},
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("the first move: %d %s", rec.Code, rec.Body)
	}

	rec = callAs(t, router, http.MethodPatch, movePath(sc.ID), apiv0.MoveWidgetsRequest{
		Version: testDraftVersion(t, mainProject),
		Moves:   []apiv0.WidgetMoveRequest{{WidgetID: created.ID, X: 50, Y: 60}},
	}, "")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", rec.Code, rec.Body)
	}
	var problem apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(problem.Detail, "Nothing to move") {
		t.Errorf("the 409 says %q", problem.Detail)
	}
}

func movePath(sceneID uuid.UUID) string {
	return mainPath + "/scenes/" + sceneID.String() + "/widgets"
}
