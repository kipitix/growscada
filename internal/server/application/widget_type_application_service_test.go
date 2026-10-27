package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// wtLibrary is the library whose Draft the widget type tests work on: a new,
// empty one after each cleanWidgetTypes.
var wtLibrary uuid.UUID

func cleanWidgetTypes(t *testing.T) {
	t.Helper()
	wtLibrary = mustCreateLibrary(t).ID
}

func newWidgetTypeService() application.WidgetTypeService {
	return application.NewWidgetTypeService(repositories.NewLibraryRepositoryPostgres(testDB), repositories.NewWidgetTypeRepositories(testDB), soleOrganization())
}

func newWidgetTypeServiceWithBus(t *testing.T) (application.WidgetTypeService, eventbus.EventBus) {
	bus, onCommit := deliveredOnCommit(t)
	return application.NewWidgetTypeService(repositories.NewLibraryRepositoryPostgres(testDB), repositories.NewWidgetTypeRepositories(testDB, onCommit), soleOrganization()), bus
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

	resp, err := svc.CreateWidgetType(context.Background(), wtLibrary, testCreateWidgetTypeInput)

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
	_, err := svc.CreateWidgetType(context.Background(), wtLibrary, input)

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
			resp, err := svc.CreateWidgetType(context.Background(), wtLibrary, input)

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

	resp, err := svc.FindAllWidgetTypes(context.Background(), wtLibrary)

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
	if _, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	if _, err := svc.CreateWidgetType(ctx, wtLibrary, input2); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	resp, err := svc.FindAllWidgetTypes(ctx, wtLibrary)

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

	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	found, err := svc.FindWidgetTypeByID(ctx, wtLibrary, created.ID)

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

	_, err := svc.FindWidgetTypeByID(context.Background(), wtLibrary, uuid.New())

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

	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	updated, err := svc.UpdateWidgetType(ctx, wtLibrary, created.ID, created.Version, appdto.WidgetTypeInput{
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

	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	_, err = svc.UpdateWidgetType(ctx, wtLibrary, created.ID, created.Version, appdto.WidgetTypeInput{
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

	found, err := svc.FindWidgetTypeByID(ctx, wtLibrary, created.ID)
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

	_, err := svc.UpdateWidgetType(context.Background(), wtLibrary, uuid.New(), 0, appdto.WidgetTypeInput{
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

	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	_, err = svc.UpdateWidgetType(ctx, wtLibrary, created.ID, created.Version, appdto.WidgetTypeInput{
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

	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	deleted, err := svc.DeleteWidgetTypeByID(ctx, wtLibrary, created.ID)

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

	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	if _, err = svc.DeleteWidgetTypeByID(ctx, wtLibrary, created.ID); err != nil {
		t.Fatalf("DeleteWidgetTypeByID: %v", err)
	}

	_, err = svc.FindWidgetTypeByID(ctx, wtLibrary, created.ID)
	if !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrWidgetTypeNotFound after delete, got: %v", err)
	}
}

func TestDeleteWidgetType_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()

	_, err := svc.DeleteWidgetTypeByID(context.Background(), wtLibrary, uuid.New())

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

	resp, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
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

	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetTypeDeleted, func(e event.Event) {
		received = append(received, e)
	})

	if _, err = svc.DeleteWidgetTypeByID(ctx, wtLibrary, created.ID); err != nil {
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

	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetTypeUpdated, func(e event.Event) {
		received = append(received, e)
	})

	_, err = svc.UpdateWidgetType(ctx, wtLibrary, created.ID, created.Version, appdto.WidgetTypeInput{
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

	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	_, err = svc.UpdateWidgetType(ctx, wtLibrary, created.ID, created.Version-1, appdto.WidgetTypeInput{
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

// TestDeleteWidgetType_UsedByWidget_TouchesNoScene: a Widget is of the copy
// the pinned release holds (ADR 0007), so removing its WidgetType from the
// Draft changes no Scene, and the release still serves new Widgets.
func TestDeleteWidgetType_UsedByWidget_TouchesNoScene(t *testing.T) {
	cleanScenes(t)
	wtSvc := newWidgetTypeService()
	sceneSvc := newSceneService()
	ctx := context.Background()

	created, lib := pinNewWidgetType(t, mainProject, testCreateWidgetTypeInput)
	sc := mustCreateScene(t, sceneSvc)
	input := testCreateWidgetInput
	input.TypeID = created.ID
	w, err := sceneSvc.CreateWidget(ctx, mainProject, sc.ID, sc.Version, input)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	if _, err := wtSvc.DeleteWidgetTypeByID(ctx, lib.ID, created.ID); err != nil {
		t.Fatalf("DeleteWidgetTypeByID: %v", err)
	}

	after, err := sceneSvc.FindSceneByID(ctx, mainProject, sc.ID)
	if err != nil || after.Version != w.SceneVersion {
		t.Errorf("the scene changed: %+v, %v", after, err)
	}
	if _, err := sceneSvc.CreateWidget(ctx, mainProject, sc.ID, after.Version, input); err != nil {
		t.Errorf("a widget of the released type after its removal from the Draft: %v", err)
	}
}

func TestWidgetType_OfAnotherOrMissingLibrary_IsNotFound(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()
	created, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	other := mustCreateLibrary(t)
	if _, err := svc.FindWidgetTypeByID(ctx, other.ID, created.ID); !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("through another library: expected ErrWidgetTypeNotFound, got %v", err)
	}
	if _, err := svc.FindAllWidgetTypes(ctx, uuid.New()); !errors.Is(err, library.ErrLibraryNotFound) {
		t.Errorf("a missing library: expected ErrLibraryNotFound, got %v", err)
	}
	if _, err := svc.CreateWidgetType(ctx, wtLibrary, testCreateWidgetTypeInput); !errors.Is(err, library.ErrWidgetTypeNameTaken) {
		t.Errorf("a taken name: expected ErrWidgetTypeNameTaken, got %v", err)
	}
}
