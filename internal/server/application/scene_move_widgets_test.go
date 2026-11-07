package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// createWidgetIn creates a widget of the shared fixture in the scene.
func createWidgetIn(t *testing.T, svc application.SceneService, sceneID uuid.UUID) appdto.Widget {
	t.Helper()
	created, err := svc.CreateWidget(context.Background(), mainProject, sceneID,
		draftVersionOf(t, mainProject), testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	return created
}

// draftChangeCountOf counts the DraftChanges of the project's Draft.
func draftChangeCountOf(t *testing.T, projectID uuid.UUID) int {
	t.Helper()
	var count int
	err := testDB.QueryRow(
		`SELECT count(*) FROM engineering.draft_changes dc JOIN engineering.drafts d ON d.id = dc.draft_id
		 WHERE d.project_id = $1`, projectID).Scan(&count)
	if err != nil {
		t.Fatalf("count the draft changes: %v", err)
	}
	return count
}

// lastChangeSides describes the latest DraftChange of the project's Draft: its
// description and the number of snapshots on each side.
func lastChangeSides(t *testing.T, projectID uuid.UUID) (string, int, int) {
	t.Helper()
	var (
		description   string
		before, after int
	)
	err := testDB.QueryRow(
		`SELECT dc.description, jsonb_array_length(dc.before->'entities'), jsonb_array_length(dc.after->'entities')
		 FROM engineering.draft_changes dc JOIN engineering.drafts d ON d.id = dc.draft_id
		 WHERE d.project_id = $1 ORDER BY dc.seq DESC LIMIT 1`, projectID).
		Scan(&description, &before, &after)
	if err != nil {
		t.Fatalf("read the last draft change: %v", err)
	}
	return description, before, after
}

// positionIn returns the x and y the service reports for the widget.
func positionIn(t *testing.T, svc application.SceneService, sceneID, widgetID uuid.UUID) (float64, float64, int) {
	t.Helper()
	w, err := svc.FindWidgetByID(context.Background(), mainProject, sceneID, widgetID)
	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}
	return w.X, w.Y, w.Z
}

func TestMoveWidgets_MovesTheGroupInOneChange(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	first := createWidgetIn(t, svc, sc.ID)
	second := createWidgetIn(t, svc, sc.ID)
	version := draftVersionOf(t, mainProject)

	got, err := svc.MoveWidgets(ctx, mainProject, sc.ID, version, []appdto.WidgetMove{
		{WidgetID: first.ID, X: 100, Y: 200},
		{WidgetID: second.ID, X: -30, Y: 40},
	})

	if err != nil {
		t.Fatalf("MoveWidgets: %v", err)
	}
	if got != version+1 {
		t.Errorf("Version: expected %d, got %d", version+1, got)
	}
	description, before, after := lastChangeSides(t, mainProject)
	if description != "Move 2 widgets" || before != 2 || after != 2 {
		t.Errorf("the change: %q with %d before and %d after snapshots", description, before, after)
	}
	for _, tt := range []struct {
		widget appdto.Widget
		x, y   float64
	}{{first, 100, 200}, {second, -30, 40}} {
		x, y, z := positionIn(t, svc, sc.ID, tt.widget.ID)
		if x != tt.x || y != tt.y || z != tt.widget.Z {
			t.Errorf("widget %s: position (%v, %v, z %d), want (%v, %v, z %d)", tt.widget.ID, x, y, z, tt.x, tt.y, tt.widget.Z)
		}
	}
	widgets, err := svc.FindWidgetsBySceneID(ctx, mainProject, sc.ID)
	if err != nil {
		t.Fatalf("FindWidgetsBySceneID: %v", err)
	}
	if len(widgets) != 2 {
		t.Errorf("the scene holds %d widgets, want 2", len(widgets))
	}
}

func TestMoveWidgets_KeepsEverythingButThePosition(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	input := testCreateWidgetInput
	input.Labels = []string{"keep", "me"}
	created, err := svc.CreateWidget(ctx, mainProject, sc.ID, draftVersionOf(t, mainProject), input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	if _, err := svc.MoveWidgets(ctx, mainProject, sc.ID, draftVersionOf(t, mainProject),
		[]appdto.WidgetMove{{WidgetID: created.ID, X: 55, Y: 66}}); err != nil {
		t.Fatalf("MoveWidgets: %v", err)
	}

	moved, err := svc.FindWidgetByID(ctx, mainProject, sc.ID, created.ID)
	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}
	if moved.X != 55 || moved.Y != 66 || moved.Z != created.Z {
		t.Errorf("position: (%v, %v, z %d), want (55, 66, z %d)", moved.X, moved.Y, moved.Z, created.Z)
	}
	if moved.Name != created.Name || moved.Width != created.Width || moved.Height != created.Height ||
		moved.OriginX != created.OriginX || moved.OriginY != created.OriginY ||
		moved.RotationDegrees != created.RotationDegrees || moved.TypeID != created.TypeID ||
		!slices.Equal(moved.Labels, created.Labels) || !slices.Equal(moved.PortBindings, created.PortBindings) {
		t.Errorf("the move changed more than the position: %+v", moved)
	}
}

func TestMoveWidgets_EmptyList_IsInvalidInput(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	version := draftVersionOf(t, mainProject)
	changes := draftChangeCountOf(t, mainProject)

	_, err := svc.MoveWidgets(ctx, mainProject, sc.ID, version, nil)

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if draftVersionOf(t, mainProject) != version || draftChangeCountOf(t, mainProject) != changes {
		t.Error("a refused move wrote the Draft")
	}
}

func TestMoveWidgets_RepeatedWidget_IsInvalidInput(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	created := createWidgetIn(t, svc, sc.ID)
	version := draftVersionOf(t, mainProject)

	_, err := svc.MoveWidgets(ctx, mainProject, sc.ID, version, []appdto.WidgetMove{
		{WidgetID: created.ID, X: 1, Y: 2},
		{WidgetID: created.ID, X: 3, Y: 4},
	})

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if draftVersionOf(t, mainProject) != version {
		t.Error("a refused move raised the Draft's Version")
	}
}

func TestMoveWidgets_UnknownScene_ReturnsNotFound(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()

	_, err := svc.MoveWidgets(context.Background(), mainProject, uuid.New(),
		draftVersionOf(t, mainProject), []appdto.WidgetMove{{WidgetID: uuid.New(), X: 1, Y: 2}})

	if !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("expected wrapped ErrSceneNotFound, got %v", err)
	}
}

func TestMoveWidgets_WidgetOfAnotherScene_ReturnsNotFound(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	target := mustCreateScene(t, svc)
	other := mustCreateScene(t, svc)
	foreign := createWidgetIn(t, svc, other.ID)

	_, err := svc.MoveWidgets(ctx, mainProject, target.ID, draftVersionOf(t, mainProject),
		[]appdto.WidgetMove{{WidgetID: foreign.ID, X: 1, Y: 2}})

	if !errors.Is(err, scene.ErrWidgetNotFound) {
		t.Errorf("expected wrapped ErrWidgetNotFound, got %v", err)
	}
}

func TestMoveWidgets_StaleVersion_ReturnsConflict(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	sc := mustCreateScene(t, svc)
	created := createWidgetIn(t, svc, sc.ID)

	_, err := svc.MoveWidgets(context.Background(), mainProject, sc.ID,
		draftVersionOf(t, mainProject)+99, []appdto.WidgetMove{{WidgetID: created.ID, X: 1, Y: 2}})

	if !errors.Is(err, draft.ErrDraftConflict) {
		t.Errorf("expected wrapped draft.ErrDraftConflict, got %v", err)
	}
}

func TestMoveWidgets_NothingToMove_IsConflictAndWritesNothing(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	first := createWidgetIn(t, svc, sc.ID)
	second := createWidgetIn(t, svc, sc.ID)
	version := draftVersionOf(t, mainProject)
	changes := draftChangeCountOf(t, mainProject)

	_, err := svc.MoveWidgets(ctx, mainProject, sc.ID, version, []appdto.WidgetMove{
		{WidgetID: first.ID, X: first.X, Y: first.Y},
		{WidgetID: second.ID, X: second.X, Y: second.Y},
	})

	if !errors.Is(err, scene.ErrNothingToMove) {
		t.Fatalf("expected ErrNothingToMove, got %v", err)
	}
	if draftVersionOf(t, mainProject) != version || draftChangeCountOf(t, mainProject) != changes {
		t.Error("a refused move wrote the Draft")
	}
}
