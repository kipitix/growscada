package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/indicator_type"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
)

func cleanIndicatorTypes(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM indicator_types"); err != nil {
		t.Fatalf("cleanIndicatorTypes: %v", err)
	}
}

func newIndicatorTypeService() application.IndicatorTypeService {
	repo := repositories.NewIndicatorTypeRepositoryPostgres(testDB)
	return application.NewIndicatorTypeService(repo, event.NewEventBus())
}

func newIndicatorTypeServiceWithBus() (application.IndicatorTypeService, event.EventBus) {
	repo := repositories.NewIndicatorTypeRepositoryPostgres(testDB)
	bus := event.NewEventBus()
	return application.NewIndicatorTypeService(repo, bus), bus
}

var testCreateInput = appdto.CreateIndicatorTypeInput{
	Name:           "gauge",
	SvgTemplate:    "<svg><circle r='10'/></svg>",
	Script:         "function render(v) { return v; }",
	ScriptLanguage: "javascript",
}

// --- CreateIndicatorType ---

func TestCreateIndicatorType_Valid_ReturnsID(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()

	resp, err := svc.CreateIndicatorType(context.Background(), testCreateInput)

	if err != nil {
		t.Fatalf("CreateIndicatorType returned unexpected error: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestCreateIndicatorType_InvalidScriptLanguage_ReturnsError(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()

	input := testCreateInput
	input.ScriptLanguage = "ruby"
	_, err := svc.CreateIndicatorType(context.Background(), input)

	if err == nil {
		t.Error("expected error for invalid script language, got nil")
	}
}

// --- FindAllIndicatorTypes ---

func TestFindAllIndicatorTypes_EmptyDB_ReturnsEmptyList(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()

	resp, err := svc.FindAllIndicatorTypes(context.Background())

	if err != nil {
		t.Fatalf("FindAllIndicatorTypes returned unexpected error: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 indicator types, got %d", len(resp))
	}
}

func TestFindAllIndicatorTypes_Multiple_ReturnsAll(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()
	ctx := context.Background()

	input2 := testCreateInput
	input2.Name = "thermometer"
	svc.CreateIndicatorType(ctx, testCreateInput) //nolint:errcheck
	svc.CreateIndicatorType(ctx, input2)          //nolint:errcheck

	resp, err := svc.FindAllIndicatorTypes(ctx)

	if err != nil {
		t.Fatalf("FindAllIndicatorTypes returned unexpected error: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 indicator types, got %d", len(resp))
	}
}

// --- FindIndicatorTypeByID ---

func TestFindIndicatorTypeByID_Existing_ReturnsCorrectFields(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()
	ctx := context.Background()

	created, err := svc.CreateIndicatorType(ctx, testCreateInput)
	if err != nil {
		t.Fatalf("CreateIndicatorType: %v", err)
	}

	id := indicator_type.NewIndicatorTypeID(indicator_type.IndicatorTypeIDWithUUID(created.ID))
	found, err := svc.FindIndicatorTypeByID(ctx, id)

	if err != nil {
		t.Fatalf("FindIndicatorTypeByID returned unexpected error: %v", err)
	}
	if found.Name != testCreateInput.Name {
		t.Errorf("Name: expected %q, got %q", testCreateInput.Name, found.Name)
	}
	if found.ScriptLanguage != testCreateInput.ScriptLanguage {
		t.Errorf("ScriptLanguage: expected %q, got %q", testCreateInput.ScriptLanguage, found.ScriptLanguage)
	}
}

func TestFindIndicatorTypeByID_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()

	_, err := svc.FindIndicatorTypeByID(context.Background(), indicator_type.NewIndicatorTypeID())

	if err == nil {
		t.Fatal("expected error for non-existent indicator type, got nil")
	}
	if !errors.Is(err, indicator_type.ErrIndicatorTypeNotFound) {
		t.Errorf("expected wrapped ErrIndicatorTypeNotFound, got: %v", err)
	}
}

// --- UpdateIndicatorType ---

func TestUpdateIndicatorType_Valid_ReturnsIncrementedVersion(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()
	ctx := context.Background()

	created, err := svc.CreateIndicatorType(ctx, testCreateInput)
	if err != nil {
		t.Fatalf("CreateIndicatorType: %v", err)
	}

	updated, err := svc.UpdateIndicatorType(ctx, appdto.UpdateIndicatorTypeInput{
		ID:             created.ID,
		Name:           "updated-gauge",
		SvgTemplate:    "<svg/>",
		Script:         "function draw() {}",
		ScriptLanguage: "python",
	})

	if err != nil {
		t.Fatalf("UpdateIndicatorType returned unexpected error: %v", err)
	}
	if updated.Version <= created.Version {
		t.Errorf("Version: expected > %d, got %d", created.Version, updated.Version)
	}
}

func TestUpdateIndicatorType_Valid_FieldsAreUpdated(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()
	ctx := context.Background()

	created, err := svc.CreateIndicatorType(ctx, testCreateInput)
	if err != nil {
		t.Fatalf("CreateIndicatorType: %v", err)
	}

	_, err = svc.UpdateIndicatorType(ctx, appdto.UpdateIndicatorTypeInput{
		ID:             created.ID,
		Name:           "new-name",
		SvgTemplate:    "<svg><rect/></svg>",
		Script:         "print('hello')",
		ScriptLanguage: "python",
	})
	if err != nil {
		t.Fatalf("UpdateIndicatorType: %v", err)
	}

	id := indicator_type.NewIndicatorTypeID(indicator_type.IndicatorTypeIDWithUUID(created.ID))
	found, err := svc.FindIndicatorTypeByID(ctx, id)
	if err != nil {
		t.Fatalf("FindIndicatorTypeByID: %v", err)
	}
	if found.Name != "new-name" {
		t.Errorf("Name: expected 'new-name', got %q", found.Name)
	}
	if found.ScriptLanguage != "python" {
		t.Errorf("ScriptLanguage: expected 'python', got %q", found.ScriptLanguage)
	}
}

func TestUpdateIndicatorType_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()

	_, err := svc.UpdateIndicatorType(context.Background(), appdto.UpdateIndicatorTypeInput{
		ID:             uuid.New(),
		Name:           "x",
		SvgTemplate:    "<svg/>",
		Script:         "x",
		ScriptLanguage: "lua",
	})

	if err == nil {
		t.Fatal("expected error for non-existent indicator type, got nil")
	}
	if !errors.Is(err, indicator_type.ErrIndicatorTypeNotFound) {
		t.Errorf("expected wrapped ErrIndicatorTypeNotFound, got: %v", err)
	}
}

func TestUpdateIndicatorType_InvalidScriptLanguage_ReturnsError(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()
	ctx := context.Background()

	created, err := svc.CreateIndicatorType(ctx, testCreateInput)
	if err != nil {
		t.Fatalf("CreateIndicatorType: %v", err)
	}

	_, err = svc.UpdateIndicatorType(ctx, appdto.UpdateIndicatorTypeInput{
		ID:             created.ID,
		Name:           "x",
		SvgTemplate:    "<svg/>",
		Script:         "x",
		ScriptLanguage: "ruby",
	})

	if err == nil {
		t.Error("expected error for invalid script language, got nil")
	}
}

// --- DeleteIndicatorTypeByID ---

func TestDeleteIndicatorType_Existing_ReturnsDeletedItem(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()
	ctx := context.Background()

	created, err := svc.CreateIndicatorType(ctx, testCreateInput)
	if err != nil {
		t.Fatalf("CreateIndicatorType: %v", err)
	}

	id := indicator_type.NewIndicatorTypeID(indicator_type.IndicatorTypeIDWithUUID(created.ID))
	deleted, err := svc.DeleteIndicatorTypeByID(ctx, id)

	if err != nil {
		t.Fatalf("DeleteIndicatorTypeByID returned unexpected error: %v", err)
	}
	if deleted.ID != created.ID {
		t.Errorf("deleted ID: expected %s, got %s", created.ID, deleted.ID)
	}
}

func TestDeleteIndicatorType_Existing_RemovedFromDB(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()
	ctx := context.Background()

	created, err := svc.CreateIndicatorType(ctx, testCreateInput)
	if err != nil {
		t.Fatalf("CreateIndicatorType: %v", err)
	}

	id := indicator_type.NewIndicatorTypeID(indicator_type.IndicatorTypeIDWithUUID(created.ID))
	if _, err = svc.DeleteIndicatorTypeByID(ctx, id); err != nil {
		t.Fatalf("DeleteIndicatorTypeByID: %v", err)
	}

	_, err = svc.FindIndicatorTypeByID(ctx, id)
	if !errors.Is(err, indicator_type.ErrIndicatorTypeNotFound) {
		t.Errorf("expected ErrIndicatorTypeNotFound after delete, got: %v", err)
	}
}

func TestDeleteIndicatorType_NotFound_ReturnsWrappedError(t *testing.T) {
	cleanIndicatorTypes(t)
	svc := newIndicatorTypeService()

	_, err := svc.DeleteIndicatorTypeByID(context.Background(), indicator_type.NewIndicatorTypeID())

	if err == nil {
		t.Fatal("expected error for non-existent indicator type, got nil")
	}
	if !errors.Is(err, indicator_type.ErrIndicatorTypeNotFound) {
		t.Errorf("expected wrapped ErrIndicatorTypeNotFound, got: %v", err)
	}
}

// --- Events ---

func TestCreateIndicatorType_Success_PublishesCreatedEvent(t *testing.T) {
	cleanIndicatorTypes(t)
	svc, bus := newIndicatorTypeServiceWithBus()
	ctx := context.Background()

	var received []event.Event
	bus.Subscribe(event.EventTypeIndicatorTypeCreated, func(e event.Event) {
		received = append(received, e)
	})

	resp, err := svc.CreateIndicatorType(ctx, testCreateInput)
	if err != nil {
		t.Fatalf("CreateIndicatorType: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	itEvent, ok := received[0].(event.IndicatorTypeEvent)
	if !ok {
		t.Fatal("expected event to implement IndicatorTypeEvent")
	}
	if itEvent.IndicatorTypeID().UUID() != resp.ID {
		t.Errorf("event IndicatorTypeID: expected %s, got %s", resp.ID, itEvent.IndicatorTypeID().UUID())
	}
}

func TestDeleteIndicatorType_Success_PublishesDeletedEvent(t *testing.T) {
	cleanIndicatorTypes(t)
	svc, bus := newIndicatorTypeServiceWithBus()
	ctx := context.Background()

	created, err := svc.CreateIndicatorType(ctx, testCreateInput)
	if err != nil {
		t.Fatalf("CreateIndicatorType: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeIndicatorTypeDeleted, func(e event.Event) {
		received = append(received, e)
	})

	id := indicator_type.NewIndicatorTypeID(indicator_type.IndicatorTypeIDWithUUID(created.ID))
	if _, err = svc.DeleteIndicatorTypeByID(ctx, id); err != nil {
		t.Fatalf("DeleteIndicatorTypeByID: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	itEvent, ok := received[0].(event.IndicatorTypeEvent)
	if !ok {
		t.Fatal("expected event to implement IndicatorTypeEvent")
	}
	if itEvent.IndicatorTypeID().UUID() != created.ID {
		t.Errorf("event IndicatorTypeID: expected %s, got %s", created.ID, itEvent.IndicatorTypeID().UUID())
	}
}

func TestUpdateIndicatorType_Success_PublishesUpdatedEvent(t *testing.T) {
	cleanIndicatorTypes(t)
	svc, bus := newIndicatorTypeServiceWithBus()
	ctx := context.Background()

	created, err := svc.CreateIndicatorType(ctx, testCreateInput)
	if err != nil {
		t.Fatalf("CreateIndicatorType: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeIndicatorTypeUpdated, func(e event.Event) {
		received = append(received, e)
	})

	_, err = svc.UpdateIndicatorType(ctx, appdto.UpdateIndicatorTypeInput{
		ID:             created.ID,
		Name:           "updated",
		SvgTemplate:    "<svg/>",
		Script:         "x",
		ScriptLanguage: "lua",
	})
	if err != nil {
		t.Fatalf("UpdateIndicatorType: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	itEvent, ok := received[0].(event.IndicatorTypeEvent)
	if !ok {
		t.Fatal("expected event to implement IndicatorTypeEvent")
	}
	if itEvent.IndicatorTypeID().UUID() != created.ID {
		t.Errorf("event IndicatorTypeID: expected %s, got %s", created.ID, itEvent.IndicatorTypeID().UUID())
	}
}
