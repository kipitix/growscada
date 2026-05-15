package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/widget"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
)

func cleanWidgetTypes(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM widget_types"); err != nil {
		t.Fatalf("cleanWidgetTypes: %v", err)
	}
}

func newWidgetTypeService() application.WidgetTypeService {
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	return application.NewWidgetTypeService(repo, event.NewEventBus())
}

func newWidgetTypeServiceWithBus() (application.WidgetTypeService, event.EventBus) {
	repo := repositories.NewWidgetTypeRepositoryPostgres(testDB)
	bus := event.NewEventBus()
	return application.NewWidgetTypeService(repo, bus), bus
}

var testCreateWidgetTypeInput = appdto.CreateWidgetTypeInput{
	Name:           "gauge",
	HtmlTemplate:   "<div class='gauge'><span class='value'></span></div>",
	Script:         "function render(v) { return v; }",
	ScriptLanguage: "javascript",
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

	if err == nil {
		t.Error("expected error for invalid script language, got nil")
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

	id := widget.NewWidgetTypeID(widget.WidgetTypeIDWithUUID(created.ID))
	found, err := svc.FindWidgetTypeByID(ctx, id)

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

	_, err := svc.FindWidgetTypeByID(context.Background(), widget.NewWidgetTypeID())

	if err == nil {
		t.Fatal("expected error for non-existent widget type, got nil")
	}
	if !errors.Is(err, widget.ErrWidgetTypeNotFound) {
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

	updated, err := svc.UpdateWidgetType(ctx, appdto.UpdateWidgetTypeInput{
		ID:             created.ID,
		Name:           "updated-gauge",
		HtmlTemplate:   "<div class='updated'></div>",
		Script:         "function draw() {}",
		ScriptLanguage: "python",
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

	_, err = svc.UpdateWidgetType(ctx, appdto.UpdateWidgetTypeInput{
		ID:             created.ID,
		Name:           "new-name",
		HtmlTemplate:   "<div class='new'></div>",
		Script:         "print('hello')",
		ScriptLanguage: "python",
	})
	if err != nil {
		t.Fatalf("UpdateWidgetType: %v", err)
	}

	id := widget.NewWidgetTypeID(widget.WidgetTypeIDWithUUID(created.ID))
	found, err := svc.FindWidgetTypeByID(ctx, id)
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

	_, err := svc.UpdateWidgetType(context.Background(), appdto.UpdateWidgetTypeInput{
		ID:             uuid.New(),
		Name:           "x",
		HtmlTemplate:   "<div/>",
		Script:         "x",
		ScriptLanguage: "lua",
	})

	if err == nil {
		t.Fatal("expected error for non-existent widget type, got nil")
	}
	if !errors.Is(err, widget.ErrWidgetTypeNotFound) {
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

	_, err = svc.UpdateWidgetType(ctx, appdto.UpdateWidgetTypeInput{
		ID:             created.ID,
		Name:           "x",
		HtmlTemplate:   "<div/>",
		Script:         "x",
		ScriptLanguage: "ruby",
	})

	if err == nil {
		t.Error("expected error for invalid script language, got nil")
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

	id := widget.NewWidgetTypeID(widget.WidgetTypeIDWithUUID(created.ID))
	deleted, err := svc.DeleteWidgetTypeByID(ctx, id)

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

	id := widget.NewWidgetTypeID(widget.WidgetTypeIDWithUUID(created.ID))
	if _, err = svc.DeleteWidgetTypeByID(ctx, id); err != nil {
		t.Fatalf("DeleteWidgetTypeByID: %v", err)
	}

	_, err = svc.FindWidgetTypeByID(ctx, id)
	if !errors.Is(err, widget.ErrWidgetTypeNotFound) {
		t.Errorf("expected ErrWidgetTypeNotFound after delete, got: %v", err)
	}
}

func TestDeleteWidgetType_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()

	_, err := svc.DeleteWidgetTypeByID(context.Background(), widget.NewWidgetTypeID())

	if err == nil {
		t.Fatal("expected error for non-existent widget type, got nil")
	}
	if !errors.Is(err, widget.ErrWidgetTypeNotFound) {
		t.Errorf("expected wrapped ErrWidgetTypeNotFound, got: %v", err)
	}
}

// --- Events ---

func TestCreateWidgetType_Success_PublishesCreatedEvent(t *testing.T) {
	cleanWidgetTypes(t)
	svc, bus := newWidgetTypeServiceWithBus()
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
	wtEvent, ok := received[0].(event.WidgetTypeEvent)
	if !ok {
		t.Fatal("expected event to implement WidgetTypeEvent")
	}
	if wtEvent.WidgetTypeID().UUID() != resp.ID {
		t.Errorf("event WidgetTypeID: expected %s, got %s", resp.ID, wtEvent.WidgetTypeID().UUID())
	}
}

func TestDeleteWidgetType_Success_PublishesDeletedEvent(t *testing.T) {
	cleanWidgetTypes(t)
	svc, bus := newWidgetTypeServiceWithBus()
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetTypeDeleted, func(e event.Event) {
		received = append(received, e)
	})

	id := widget.NewWidgetTypeID(widget.WidgetTypeIDWithUUID(created.ID))
	if _, err = svc.DeleteWidgetTypeByID(ctx, id); err != nil {
		t.Fatalf("DeleteWidgetTypeByID: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	wtEvent, ok := received[0].(event.WidgetTypeEvent)
	if !ok {
		t.Fatal("expected event to implement WidgetTypeEvent")
	}
	if wtEvent.WidgetTypeID().UUID() != created.ID {
		t.Errorf("event WidgetTypeID: expected %s, got %s", created.ID, wtEvent.WidgetTypeID().UUID())
	}
}

func TestUpdateWidgetType_Success_PublishesUpdatedEvent(t *testing.T) {
	cleanWidgetTypes(t)
	svc, bus := newWidgetTypeServiceWithBus()
	ctx := context.Background()

	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeWidgetTypeUpdated, func(e event.Event) {
		received = append(received, e)
	})

	_, err = svc.UpdateWidgetType(ctx, appdto.UpdateWidgetTypeInput{
		ID:             created.ID,
		Name:           "updated",
		HtmlTemplate:   "<div/>",
		Script:         "x",
		ScriptLanguage: "lua",
	})
	if err != nil {
		t.Fatalf("UpdateWidgetType: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	wtEvent, ok := received[0].(event.WidgetTypeEvent)
	if !ok {
		t.Fatal("expected event to implement WidgetTypeEvent")
	}
	if wtEvent.WidgetTypeID().UUID() != created.ID {
		t.Errorf("event WidgetTypeID: expected %s, got %s", created.ID, wtEvent.WidgetTypeID().UUID())
	}
}
