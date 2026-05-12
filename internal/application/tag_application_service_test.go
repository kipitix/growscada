package application_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/application/app_dto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/infrastructure/postgres/repositories"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		panic("failed to start postgres container: " + err.Error())
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		panic("failed to get connection string: " + err.Error())
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		panic("failed to open db: " + err.Error())
	}

	_, currentFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(currentFile), "..", "infrastructure", "postgres", "migrations")

	if err := goose.SetDialect("postgres"); err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		panic("failed to set goose dialect: " + err.Error())
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		panic("failed to run migrations: " + err.Error())
	}

	testDB = db

	code := m.Run()

	db.Close()
	pgContainer.Terminate(ctx)
	os.Exit(code)
}

func cleanTags(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM tags"); err != nil {
		t.Fatalf("cleanTags: %v", err)
	}
}

func newService() application.TagService {
	repo := repositories.NewTagRepositoryPostgres(testDB)
	return application.NewTagService(repo, event.NewEventBus())
}

func newServiceWithBus() (application.TagService, event.EventBus) {
	repo := repositories.NewTagRepositoryPostgres(testDB)
	bus := event.NewEventBus()
	return application.NewTagService(repo, bus), bus
}

// --- CreateTag ---

func TestCreateTag_ValidIntegerTag_ReturnsID(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := app_dto.CreateTagInput{Name: "temperature", Type: "integer", Value: "42", Quality: "good"}
	resp, err := svc.CreateTag(ctx, req)

	if err != nil {
		t.Fatalf("CreateTag returned unexpected error: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestCreateTag_ValidStringTag_ReturnsID(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := app_dto.CreateTagInput{Name: "label", Type: "string", Value: "hello", Quality: "good"}
	resp, err := svc.CreateTag(ctx, req)

	if err != nil {
		t.Fatalf("CreateTag returned unexpected error: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestCreateTag_ValidBooleanTag_ReturnsID(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := app_dto.CreateTagInput{Name: "enabled", Type: "boolean", Value: "true", Quality: "good"}
	resp, err := svc.CreateTag(ctx, req)

	if err != nil {
		t.Fatalf("CreateTag returned unexpected error: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestCreateTag_InvalidType_ReturnsError(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := app_dto.CreateTagInput{Name: "temperature", Type: "unknown", Value: "42", Quality: "good"}
	_, err := svc.CreateTag(ctx, req)

	if err == nil {
		t.Error("expected error for invalid type, got nil")
	}
}

func TestCreateTag_InvalidQuality_ReturnsError(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := app_dto.CreateTagInput{Name: "temperature", Type: "integer", Value: "42", Quality: "unknown"}
	_, err := svc.CreateTag(ctx, req)

	if err == nil {
		t.Error("expected error for invalid quality, got nil")
	}
}

func TestCreateTag_InvalidValueForType_ReturnsError(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := app_dto.CreateTagInput{Name: "temperature", Type: "integer", Value: "not-a-number", Quality: "good"}
	_, err := svc.CreateTag(ctx, req)

	if err == nil {
		t.Error("expected error for value incompatible with type, got nil")
	}
}

// --- FindAllTags ---

func TestFindAllTags_EmptyDB_ReturnsEmptyList(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	resp, err := svc.FindAllTags(ctx)

	if err != nil {
		t.Fatalf("FindAllTags returned unexpected error: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 tags, got %d", len(resp))
	}
}

func TestFindAllTags_MultipleTags_ReturnsAll(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req1 := app_dto.CreateTagInput{Name: "temperature", Type: "integer", Value: "10", Quality: "good"}
	req2 := app_dto.CreateTagInput{Name: "pressure", Type: "integer", Value: "20", Quality: "good"}
	if _, err := svc.CreateTag(ctx, req1); err != nil {
		t.Fatalf("CreateTag(temperature): %v", err)
	}
	if _, err := svc.CreateTag(ctx, req2); err != nil {
		t.Fatalf("CreateTag(pressure): %v", err)
	}

	resp, err := svc.FindAllTags(ctx)

	if err != nil {
		t.Fatalf("FindAllTags returned unexpected error: %v", err)
	}
	if len(resp) != 2 {
		t.Errorf("expected 2 tags, got %d", len(resp))
	}
}

// --- FindTagByID ---

func TestFindTagByID_ExistingTag_ReturnsTag(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := app_dto.CreateTagInput{Name: "humidity", Type: "integer", Value: "55", Quality: "good"}
	createResp, err := svc.CreateTag(ctx, req)
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	tagID := tag.NewTagID(tag.TagIDWithUUID(createResp.ID))
	found, err := svc.FindTagByID(ctx, tagID)

	if err != nil {
		t.Fatalf("FindTagByID returned unexpected error: %v", err)
	}
	if found.Name != "humidity" {
		t.Errorf("Name: expected 'humidity', got %q", found.Name)
	}
	if found.Type != "integer" {
		t.Errorf("Type:expected 'integer', got %q", found.Type)
	}
	if found.Value != "55" {
		t.Errorf("Value: expected '55', got %q", found.Value)
	}
	if found.Quality != "good" {
		t.Errorf("Quality: expected 'good', got %q", found.Quality)
	}
}

func TestFindTagByID_NotFound_ReturnsWrappedErrTagNotFound(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	nonExistentID := tag.NewTagID()
	_, err := svc.FindTagByID(ctx, nonExistentID)

	if err == nil {
		t.Fatal("expected error for non-existent tag, got nil")
	}
	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected wrapped ErrTagNotFound, got: %v", err)
	}
}

// --- DeleteTagByID ---

func TestDeleteTag_ExistingTag_ReturnsDeletedTag(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "sensor", Type: "integer", Value: "10", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	tagID := tag.NewTagID(tag.TagIDWithUUID(created.ID))
	resp, err := svc.DeleteTagByID(ctx, tagID)

	if err != nil {
		t.Fatalf("DeleteTagByID returned unexpected error: %v", err)
	}
	if resp.ID != created.ID {
		t.Errorf("deleted tag ID: expected %s, got %s", created.ID, resp.ID)
	}
	if resp.Name != "sensor" {
		t.Errorf("deleted tag Name: expected 'sensor', got %q", resp.Name)
	}
}

func TestDeleteTag_ExistingTag_TagIsRemovedFromDB(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "valve", Type: "boolean", Value: "true", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	tagID := tag.NewTagID(tag.TagIDWithUUID(created.ID))
	if _, err = svc.DeleteTagByID(ctx, tagID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}

	_, err = svc.FindTagByID(ctx, tagID)
	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected ErrTagNotFound after delete, got: %v", err)
	}
}

func TestDeleteTag_NotFound_ReturnsWrappedErrTagNotFound(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	_, err := svc.DeleteTagByID(ctx, tag.NewTagID())

	if err == nil {
		t.Fatal("expected error for non-existent tag, got nil")
	}
	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected wrapped ErrTagNotFound, got: %v", err)
	}
}

// --- SetTagValueByID ---

func TestSetTagValueByID_ValidUpdate_ReturnsIncrementedVersion(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "pressure", Type: "integer", Value: "100", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	resp, err := svc.SetTagValueByID(ctx, app_dto.UpdateTagInput{ID: created.ID, Value: "200", Quality: "good"})

	if err != nil {
		t.Fatalf("SetTagValueByID returned unexpected error: %v", err)
	}
	if resp.Version != 2 {
		t.Errorf("Version: expected 2, got %d", resp.Version)
	}
}

func TestSetTagValueByID_ValidUpdate_ValueAndQualityAreUpdated(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "flow", Type: "integer", Value: "0", Quality: "bad"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	tagID := tag.NewTagID(tag.TagIDWithUUID(created.ID))
	if _, err = svc.SetTagValueByID(ctx, app_dto.UpdateTagInput{ID: created.ID, Value: "42", Quality: "good"}); err != nil {
		t.Fatalf("SetTagValueByID: %v", err)
	}

	found, err := svc.FindTagByID(ctx, tagID)
	if err != nil {
		t.Fatalf("FindTagByID: %v", err)
	}
	if found.Value != "42" {
		t.Errorf("Value: expected '42', got %q", found.Value)
	}
	if found.Quality != "good" {
		t.Errorf("Quality: expected 'good', got %q", found.Quality)
	}
}

func TestSetTagValueByID_NotFound_ReturnsWrappedErrTagNotFound(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	_, err := svc.SetTagValueByID(ctx, app_dto.UpdateTagInput{ID: uuid.New(), Value: "1", Quality: "good"})

	if err == nil {
		t.Fatal("expected error for non-existent tag, got nil")
	}
	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected wrapped ErrTagNotFound, got: %v", err)
	}
}

func TestSetTagValueByID_InvalidQuality_ReturnsError(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "temp", Type: "integer", Value: "10", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	_, err = svc.SetTagValueByID(ctx, app_dto.UpdateTagInput{ID: created.ID, Value: "20", Quality: "unknown"})

	if err == nil {
		t.Error("expected error for invalid quality, got nil")
	}
}

// --- Events ---

func TestCreateTag_Success_PublishesTagCreatedEvent(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus()
	ctx := context.Background()

	var received []event.Event
	bus.Subscribe(event.EventTypeTagCreated, func(e event.Event) {
		received = append(received, e)
	})

	resp, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "sensor", Type: "integer", Value: "1", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	tagEvent, ok := received[0].(event.TagEvent)
	if !ok {
		t.Fatal("expected event to implement TagEvent")
	}
	if tagEvent.TagID().UUID() != resp.ID {
		t.Errorf("event TagID: expected %s, got %s", resp.ID, tagEvent.TagID().UUID())
	}
}

func TestCreateTag_InvalidRequest_NoEventPublished(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus()
	ctx := context.Background()

	var received []event.Event
	bus.Subscribe(event.EventTypeTagCreated, func(e event.Event) {
		received = append(received, e)
	})

	_, _ = svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "sensor", Type: "unknown", Value: "1", Quality: "good"})

	if len(received) != 0 {
		t.Errorf("expected no events on error, got %d", len(received))
	}
}

func TestDeleteTagByID_Success_PublishesTagDeletedEvent(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "valve", Type: "boolean", Value: "true", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeTagDeleted, func(e event.Event) {
		received = append(received, e)
	})

	tagID := tag.NewTagID(tag.TagIDWithUUID(created.ID))
	if _, err = svc.DeleteTagByID(ctx, tagID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	tagEvent, ok := received[0].(event.TagEvent)
	if !ok {
		t.Fatal("expected event to implement TagEvent")
	}
	if tagEvent.TagID().UUID() != created.ID {
		t.Errorf("event TagID: expected %s, got %s", created.ID, tagEvent.TagID().UUID())
	}
}

func TestDeleteTagByID_NotFound_NoEventPublished(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus()
	ctx := context.Background()

	var received []event.Event
	bus.Subscribe(event.EventTypeTagDeleted, func(e event.Event) {
		received = append(received, e)
	})

	_, _ = svc.DeleteTagByID(ctx, tag.NewTagID())

	if len(received) != 0 {
		t.Errorf("expected no events on error, got %d", len(received))
	}
}

func TestSetTagValueByID_Success_PublishesTagUpdatedEvent(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "pressure", Type: "integer", Value: "10", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeTagUpdated, func(e event.Event) {
		received = append(received, e)
	})

	if _, err = svc.SetTagValueByID(ctx, app_dto.UpdateTagInput{ID: created.ID, Value: "20", Quality: "good"}); err != nil {
		t.Fatalf("SetTagValueByID: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	tagEvent, ok := received[0].(event.TagEvent)
	if !ok {
		t.Fatal("expected event to implement TagEvent")
	}
	if tagEvent.TagID().UUID() != created.ID {
		t.Errorf("event TagID: expected %s, got %s", created.ID, tagEvent.TagID().UUID())
	}
}

func TestSetTagValueByID_InvalidRequest_NoEventPublished(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "flow", Type: "integer", Value: "0", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeTagUpdated, func(e event.Event) {
		received = append(received, e)
	})

	_, _ = svc.SetTagValueByID(ctx, app_dto.UpdateTagInput{ID: created.ID, Value: "not-a-number", Quality: "good"})

	if len(received) != 0 {
		t.Errorf("expected no events on error, got %d", len(received))
	}
}

func TestSetTagValueByID_InvalidValueForType_ReturnsError(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, app_dto.CreateTagInput{Name: "counter", Type: "integer", Value: "0", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	_, err = svc.SetTagValueByID(ctx, app_dto.UpdateTagInput{ID: created.ID, Value: "not-a-number", Quality: "good"})

	if err == nil {
		t.Error("expected error for value incompatible with type, got nil")
	}
}
