package application_test

import (
	"context"
	"errors"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

func cleanWidgetTypes(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM widgets; DELETE FROM widget_types"); err != nil {
		t.Fatalf("cleanWidgetTypes: %v", err)
	}
}

func newWidgetTypeService() application.WidgetTypeService {
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB)
	return application.NewWidgetTypeService(repo, sceneRepo)
}

func newWidgetTypeServiceWithBus(t *testing.T) (application.WidgetTypeService, eventbus.EventBus) {
	bus, onCommit := deliveredOnCommit(t)
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB, onCommit)
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB, onCommit)
	return application.NewWidgetTypeService(repo, sceneRepo), bus
}

var testCreateWidgetTypeInput = appdto.WidgetTypeInput{
	Name:           "gauge",
	HtmlTemplate:   "<div class='gauge'><span class='value'></span></div>",
	Script:         "function render(v) { return v; }",
	ScriptLanguage: "javascript",
	DefaultWidth:   120,
	DefaultHeight:  60,
}

// --- CreateWidgetType ---

func TestCreateWidgetType_Valid_ReturnsID(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()

	resp, err := svc.CreateWidgetType(context.Background(), testCreateWidgetTypeInput)

	if err != nil {
		t.Fatalf("CreateWidgetType returned unexpected error: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestCreateWidgetType_InvalidScriptLanguage_ReturnsError(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()

	input := testCreateWidgetTypeInput
	input.ScriptLanguage = "ruby"
	_, err := svc.CreateWidgetType(context.Background(), input)

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid script language, got: %v", err)
	}
}

func TestCreateWidgetType_InputPortTypeHint(t *testing.T) {
	cases := []struct {
		typeHint string
		wantErr  bool
	}{
		{"", false},
		{"integer", false},
		{"unknown", true},
	}

	for _, tc := range cases {
		t.Run(tc.typeHint, func(t *testing.T) {
			cleanWidgetTypes(t)
			svc := newWidgetTypeService()

			input := testCreateWidgetTypeInput
			input.InputPorts = []appdto.InputPort{{Name: "value", TypeHint: tc.typeHint}}
			resp, err := svc.CreateWidgetType(context.Background(), input)

			if (err != nil) != tc.wantErr {
				t.Fatalf("CreateWidgetType error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got := resp.InputPorts[0].TypeHint; got != tc.typeHint {
				t.Errorf("TypeHint: expected %q, got %q", tc.typeHint, got)
			}
		})
	}
}

// --- FindAllWidgetTypes ---

func TestFindAllWidgetTypes_EmptyDB_ReturnsEmptyList(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()

	resp, err := svc.FindAllWidgetTypes(context.Background())

	if err != nil {
		t.Fatalf("FindAllWidgetTypes returned unexpected error: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 widget types, got %d", len(resp))
	}
}

func TestFindAllWidgetTypes_Multiple_ReturnsAll(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()

	input2 := testCreateWidgetTypeInput
	input2.Name = "thermometer"
	if _, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	if _, err := svc.CreateWidgetType(ctx, input2); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	resp, err := svc.FindAllWidgetTypes(ctx)

	if err != nil {
		t.Fatalf("FindAllWidgetTypes returned unexpected error: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 widget types, got %d", len(resp))
	}
}

// --- FindWidgetTypeByID ---

func TestFindWidgetTypeByID_Existing_ReturnsCorrectFields(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	found, err := svc.FindWidgetTypeByID(ctx, created.ID)

	if err != nil {
		t.Fatalf("FindWidgetTypeByID returned unexpected error: %v", err)
	}
	if found.Name != testCreateWidgetTypeInput.Name {
		t.Errorf("Name: expected %q, got %q", testCreateWidgetTypeInput.Name, found.Name)
	}
	if found.ScriptLanguage != testCreateWidgetTypeInput.ScriptLanguage {
		t.Errorf("ScriptLanguage: expected %q, got %q", testCreateWidgetTypeInput.ScriptLanguage, found.ScriptLanguage)
	}
}

func TestFindWidgetTypeByID_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()

	_, err := svc.FindWidgetTypeByID(context.Background(), uuid.New())

	if err == nil {
		t.Fatal("expected error for non-existent widget type, got nil")
	}
	if !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected wrapped ErrWidgetTypeNotFound, got: %v", err)
	}
}

// --- UpdateWidgetType ---

func TestUpdateWidgetType_Valid_ReturnsIncrementedVersion(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	updated, err := svc.UpdateWidgetType(ctx, created.ID, created.Version, appdto.WidgetTypeInput{
		Name:           "updated-gauge",
		HtmlTemplate:   "<div class='updated'></div>",
		Script:         "function draw() {}",
		ScriptLanguage: "python",
		DefaultWidth:   120,
		DefaultHeight:  60,
	})

	if err != nil {
		t.Fatalf("UpdateWidgetType returned unexpected error: %v", err)
	}
	if updated.Version <= created.Version {
		t.Errorf("Version: expected > %d, got %d", created.Version, updated.Version)
	}
}

func TestUpdateWidgetType_Valid_FieldsAreUpdated(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	_, err = svc.UpdateWidgetType(ctx, created.ID, created.Version, appdto.WidgetTypeInput{
		Name:           "new-name",
		HtmlTemplate:   "<div class='new'></div>",
		Script:         "print('hello')",
		ScriptLanguage: "python",
		DefaultWidth:   120,
		DefaultHeight:  60,
	})
	if err != nil {
		t.Fatalf("UpdateWidgetType: %v", err)
	}

	found, err := svc.FindWidgetTypeByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindWidgetTypeByID: %v", err)
	}
	if found.Name != "new-name" {
		t.Errorf("Name: expected 'new-name', got %q", found.Name)
	}
	if found.ScriptLanguage != "python" {
		t.Errorf("ScriptLanguage: expected 'python', got %q", found.ScriptLanguage)
	}
}

func TestUpdateWidgetType_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()

	_, err := svc.UpdateWidgetType(context.Background(), uuid.New(), 0, appdto.WidgetTypeInput{
		Name:           "x",
		HtmlTemplate:   "<div/>",
		Script:         "x",
		ScriptLanguage: "lua",
	})

	if err == nil {
		t.Fatal("expected error for non-existent widget type, got nil")
	}
	if !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected wrapped ErrWidgetTypeNotFound, got: %v", err)
	}
}

func TestUpdateWidgetType_InvalidScriptLanguage_ReturnsError(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	_, err = svc.UpdateWidgetType(ctx, created.ID, created.Version, appdto.WidgetTypeInput{
		Name:           "x",
		HtmlTemplate:   "<div/>",
		Script:         "x",
		ScriptLanguage: "ruby",
		DefaultWidth:   120,
		DefaultHeight:  60,
	})

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid script language, got: %v", err)
	}
}

// --- DeleteWidgetTypeByID ---

func TestDeleteWidgetType_Existing_ReturnsDeletedItem(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	deleted, err := svc.DeleteWidgetTypeByID(ctx, created.ID)

	if err != nil {
		t.Fatalf("DeleteWidgetTypeByID returned unexpected error: %v", err)
	}
	if deleted.ID != created.ID {
		t.Errorf("deleted ID: expected %s, got %s", created.ID, deleted.ID)
	}
}

func TestDeleteWidgetType_Existing_RemovedFromDB(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	if _, err = svc.DeleteWidgetTypeByID(ctx, created.ID); err != nil {
		t.Fatalf("DeleteWidgetTypeByID: %v", err)
	}

	_, err = svc.FindWidgetTypeByID(ctx, created.ID)
	if !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrWidgetTypeNotFound after delete, got: %v", err)
	}
}

func TestDeleteWidgetType_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()

	_, err := svc.DeleteWidgetTypeByID(context.Background(), uuid.New())

	if err == nil {
		t.Fatal("expected error for non-existent widget type, got nil")
	}
	if !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected wrapped ErrWidgetTypeNotFound, got: %v", err)
	}
}

// --- Events ---

func TestCreateWidgetType_Success_PublishesCreatedEvent(t *testing.T) {
	cleanWidgetTypes(t)
	svc, bus := newWidgetTypeServiceWithBus(t)
	ctx := context.Background()

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetTypeCreated, func(e event.Event) {
		received = append(received, e)
	})

	resp, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	wtEvent, ok := received[0].(library.WidgetTypeEvent)
	if !ok {
		t.Fatal("expected event to implement WidgetTypeEvent")
	}
	if wtEvent.WidgetTypeID().UUID() != resp.ID {
		t.Errorf("event WidgetTypeID: expected %s, got %s", resp.ID, wtEvent.WidgetTypeID().UUID())
	}
}

func TestDeleteWidgetType_Success_PublishesDeletedEvent(t *testing.T) {
	cleanWidgetTypes(t)
	svc, bus := newWidgetTypeServiceWithBus(t)
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetTypeDeleted, func(e event.Event) {
		received = append(received, e)
	})

	if _, err = svc.DeleteWidgetTypeByID(ctx, created.ID); err != nil {
		t.Fatalf("DeleteWidgetTypeByID: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	wtEvent, ok := received[0].(library.WidgetTypeEvent)
	if !ok {
		t.Fatal("expected event to implement WidgetTypeEvent")
	}
	if wtEvent.WidgetTypeID().UUID() != created.ID {
		t.Errorf("event WidgetTypeID: expected %s, got %s", created.ID, wtEvent.WidgetTypeID().UUID())
	}
}

func TestUpdateWidgetType_Success_PublishesUpdatedEvent(t *testing.T) {
	cleanWidgetTypes(t)
	svc, bus := newWidgetTypeServiceWithBus(t)
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetTypeUpdated, func(e event.Event) {
		received = append(received, e)
	})

	_, err = svc.UpdateWidgetType(ctx, created.ID, created.Version, appdto.WidgetTypeInput{
		Name:           "updated",
		HtmlTemplate:   "<div/>",
		Script:         "x",
		ScriptLanguage: "lua",
		DefaultWidth:   120,
		DefaultHeight:  60,
	})
	if err != nil {
		t.Fatalf("UpdateWidgetType: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	wtEvent, ok := received[0].(library.WidgetTypeEvent)
	if !ok {
		t.Fatal("expected event to implement WidgetTypeEvent")
	}
	if wtEvent.WidgetTypeID().UUID() != created.ID {
		t.Errorf("event WidgetTypeID: expected %s, got %s", created.ID, wtEvent.WidgetTypeID().UUID())
	}
}

func TestUpdateWidgetType_StaleVersion_ReturnsConflict(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	_, err = svc.UpdateWidgetType(ctx, created.ID, created.Version-1, appdto.WidgetTypeInput{
		Name:           "x",
		HtmlTemplate:   "<div/>",
		Script:         "x",
		ScriptLanguage: "lua",
		DefaultWidth:   120,
		DefaultHeight:  60,
		// intentionally stale
	})

	if err == nil {
		t.Fatal("expected ErrWidgetTypeConflict for stale version, got nil")
	}
	if !errors.Is(err, library.ErrWidgetTypeConflict) {
		t.Errorf("expected wrapped ErrWidgetTypeConflict, got: %v", err)
	}
}

// --- removeOrphanedPortBindings ---

// TestUpdateWidgetType_RemovedPort_CleansBindingsOnAllWidgetsInSameScene is the
// regression test for a bug where removeOrphanedPortBindings reused the same
// (now-stale) scene version for every widget it touched: with 2+ widgets of
// the same type in one scene, the second widget's cleanup update failed with a
// spurious scene.ErrSceneConflict because the first update had already bumped
// the scene's version in the DB.
func TestUpdateWidgetType_RemovedPort_CleansBindingsOnAllWidgetsInSameScene(t *testing.T) {
	cleanScenes(t)
	cleanWidgetTypes(t)
	wtSvc := newWidgetTypeService()
	sceneSvc := newSceneService()
	ctx := context.Background()

	createdType, err := wtSvc.CreateWidgetType(ctx, appdto.WidgetTypeInput{
		Name:           "sensor",
		HtmlTemplate:   "<div></div>",
		Script:         "function render(v) {}",
		ScriptLanguage: "javascript",
		DefaultWidth:   100,
		DefaultHeight:  100,
		InputPorts:     []appdto.InputPort{{Name: "value"}},
	})
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	sc := mustCreateScene(t, sceneSvc)

	widgetInput := appdto.WidgetInput{
		Name:         "widget-1",
		Width:        100,
		Height:       100,
		OriginX:      0.5,
		OriginY:      0.5,
		TypeID:       createdType.ID,
		PortBindings: []appdto.PortBinding{{PortName: "value", TagID: uuid.New()}},
	}
	first, err := sceneSvc.CreateWidget(ctx, sc.ID, sc.Version, widgetInput)
	if err != nil {
		t.Fatalf("CreateWidget 1: %v", err)
	}

	widgetInput.Name = "widget-2"
	second, err := sceneSvc.CreateWidget(ctx, sc.ID, first.SceneVersion, widgetInput)
	if err != nil {
		t.Fatalf("CreateWidget 2: %v", err)
	}

	// Drop the "value" port from the type: both widgets' bindings to it are
	// now orphaned and must be cleaned up by the same UpdateWidgetType call.
	_, err = wtSvc.UpdateWidgetType(ctx, createdType.ID, createdType.Version, appdto.WidgetTypeInput{
		Name:           createdType.Name,
		HtmlTemplate:   createdType.HtmlTemplate,
		Script:         createdType.Script,
		ScriptLanguage: createdType.ScriptLanguage,
		DefaultWidth:   createdType.DefaultWidth,
		DefaultHeight:  createdType.DefaultHeight,
		InputPorts:     nil,
	})
	if err != nil {
		t.Fatalf("UpdateWidgetType: %v", err)
	}

	w1, err := sceneSvc.FindWidgetByID(ctx, sc.ID, first.ID)
	if err != nil {
		t.Fatalf("FindWidgetByID 1: %v", err)
	}
	if len(w1.PortBindings) != 0 {
		t.Errorf("widget 1: expected orphaned port bindings removed, got %v", w1.PortBindings)
	}

	w2, err := sceneSvc.FindWidgetByID(ctx, sc.ID, second.ID)
	if err != nil {
		t.Fatalf("FindWidgetByID 2: %v", err)
	}
	if len(w2.PortBindings) != 0 {
		t.Errorf("widget 2: expected orphaned port bindings removed, got %v", w2.PortBindings)
	}
}

// listRacingSceneRepository runs race once, right after the first
// FindByWidgetTypeID: someone else changes a listed Scene before the cleanup
// saves it.
type listRacingSceneRepository struct {
	scene.SceneRepository
	race func()
}

func (r *listRacingSceneRepository) FindByWidgetTypeID(ctx context.Context, typeID id.ID[library.WidgetType]) ([]scene.Scene, error) {
	found, err := r.SceneRepository.FindByWidgetTypeID(ctx, typeID)
	if race := r.race; race != nil {
		r.race = nil
		race()
	}
	return found, err
}

func TestUpdateWidgetType_RemovedPort_SceneChangedMeanwhile_CleansBindingsKeepsChange(t *testing.T) {
	cleanScenes(t)
	cleanWidgetTypes(t)
	wtSvc := newWidgetTypeService()
	sceneSvc := newSceneService()
	ctx := context.Background()

	createdType, err := wtSvc.CreateWidgetType(ctx, appdto.WidgetTypeInput{
		Name:           "sensor",
		HtmlTemplate:   "<div></div>",
		Script:         "function render(v) {}",
		ScriptLanguage: "javascript",
		DefaultWidth:   100,
		DefaultHeight:  100,
		InputPorts:     []appdto.InputPort{{Name: "value"}},
	})
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	sc := mustCreateScene(t, sceneSvc)
	created, err := sceneSvc.CreateWidget(ctx, sc.ID, sc.Version, appdto.WidgetInput{
		Name:         "widget-1",
		Width:        100,
		Height:       100,
		OriginX:      0.5,
		OriginY:      0.5,
		TypeID:       createdType.ID,
		PortBindings: []appdto.PortBinding{{PortName: "value", TagID: uuid.New()}},
	})
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	sceneRepo := &listRacingSceneRepository{SceneRepository: repositories.NewSceneRepositoryPostgres(testDB)}
	sceneRepo.race = func() {
		if _, err := sceneSvc.UpdateScene(ctx, sc.ID, created.SceneVersion, appdto.SceneInput{Name: "renamed", Width: 800, Height: 600}); err != nil {
			t.Fatalf("concurrent UpdateScene: %v", err)
		}
	}
	racingSvc := application.NewWidgetTypeService(repositories.NewWidgetTypeRepositoryPostgres(testDB), sceneRepo)

	_, err = racingSvc.UpdateWidgetType(ctx, createdType.ID, createdType.Version, appdto.WidgetTypeInput{
		Name:           createdType.Name,
		HtmlTemplate:   createdType.HtmlTemplate,
		Script:         createdType.Script,
		ScriptLanguage: createdType.ScriptLanguage,
		DefaultWidth:   createdType.DefaultWidth,
		DefaultHeight:  createdType.DefaultHeight,
		InputPorts:     nil,
	})

	if err != nil {
		t.Fatalf("UpdateWidgetType: %v", err)
	}
	found, err := sceneSvc.FindSceneByID(ctx, sc.ID)
	if err != nil {
		t.Fatalf("FindSceneByID: %v", err)
	}
	if found.Name != "renamed" {
		t.Errorf("expected the concurrent change kept, got name %q", found.Name)
	}
	w, err := sceneSvc.FindWidgetByID(ctx, sc.ID, created.ID)
	if err != nil {
		t.Fatalf("FindWidgetByID: %v", err)
	}
	if len(w.PortBindings) != 0 {
		t.Errorf("expected orphaned port bindings removed, got %v", w.PortBindings)
	}
}

func TestDeleteWidgetType_UsedByWidget_ReturnsErrWidgetTypeInUse(t *testing.T) {
	cleanScenes(t)
	cleanWidgetTypes(t)
	wtSvc := newWidgetTypeService()
	sceneSvc := newSceneService()
	ctx := context.Background()

	created, err := wtSvc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	sc := mustCreateScene(t, sceneSvc)
	input := testCreateWidgetInput
	input.TypeID = created.ID
	if _, err := sceneSvc.CreateWidget(ctx, sc.ID, sc.Version, input); err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	_, err = wtSvc.DeleteWidgetTypeByID(ctx, created.ID)

	if !errors.Is(err, library.ErrWidgetTypeInUse) {
		t.Errorf("expected wrapped ErrWidgetTypeInUse, got: %v", err)
	}
	if _, err := wtSvc.FindWidgetTypeByID(ctx, created.ID); err != nil {
		t.Errorf("widget type after refused delete: %v", err)
	}
}

// outboxSceneIDs returns, per event type, the Scenes the outbox's events are
// about: a Scene event's own, a Widget event's holder.
func outboxSceneIDs(t *testing.T) map[string][]uuid.UUID {
	t.Helper()
	rows, err := testDB.QueryContext(context.Background(), "SELECT type, payload FROM outbox ORDER BY seq")
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer rows.Close()
	out := map[string][]uuid.UUID{}
	for rows.Next() {
		var typeName string
		var data []byte
		if err := rows.Scan(&typeName, &data); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		et, err := event.NewEventType(typeName)
		if err != nil {
			t.Fatalf("event type: %v", err)
		}
		e, err := outbox.Decode(et, data)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch ev := e.(type) {
		case scene.WidgetEvent:
			out[typeName] = append(out[typeName], ev.SceneID().UUID())
		case scene.SceneEvent:
			out[typeName] = append(out[typeName], ev.SceneID().UUID())
		}
	}
	return out
}

func TestUpdateWidgetType_RemovedPort_OutboxHoldsEventsOfEveryChangedScene(t *testing.T) {
	cleanScenes(t)
	cleanWidgetTypes(t)
	wtSvc := newWidgetTypeService()
	sceneSvc := newSceneService()
	ctx := context.Background()

	input := appdto.WidgetTypeInput{
		Name: "sensor", HtmlTemplate: "<div></div>", ScriptLanguage: "javascript",
		DefaultWidth: 100, DefaultHeight: 100, InputPorts: []appdto.InputPort{{Name: "value"}},
	}
	createdType, err := wtSvc.CreateWidgetType(ctx, input)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	bound := appdto.WidgetInput{
		Name: "bound", Width: 100, Height: 100, OriginX: 0.5, OriginY: 0.5, TypeID: createdType.ID,
		PortBindings: []appdto.PortBinding{{PortName: "value", TagID: uuid.New()}},
	}
	unbound := bound
	unbound.PortBindings = nil

	var changedScenes []uuid.UUID
	for _, name := range []string{"first", "second"} {
		sc, err := sceneSvc.CreateScene(ctx, appdto.SceneInput{Name: name, Width: 800, Height: 600})
		if err != nil {
			t.Fatalf("CreateScene: %v", err)
		}
		if _, err := sceneSvc.CreateWidget(ctx, sc.ID, sc.Version, bound); err != nil {
			t.Fatalf("CreateWidget: %v", err)
		}
		changedScenes = append(changedScenes, sc.ID)
	}
	untouched, err := sceneSvc.CreateScene(ctx, appdto.SceneInput{Name: "untouched", Width: 800, Height: 600})
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	if _, err := sceneSvc.CreateWidget(ctx, untouched.ID, untouched.Version, unbound); err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	cleanOutbox(t)

	input.InputPorts = nil
	if _, err := wtSvc.UpdateWidgetType(ctx, createdType.ID, createdType.Version, input); err != nil {
		t.Fatalf("UpdateWidgetType: %v", err)
	}

	got := outboxSceneIDs(t)
	if !slices.Equal(got["widget_updated"], changedScenes) {
		t.Errorf("widget_updated in the outbox for scenes %v, want each changed scene %v", got["widget_updated"], changedScenes)
	}
}
