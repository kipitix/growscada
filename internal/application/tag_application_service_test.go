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
	"github.com/kipitix/growscada/internal/application/dto"
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
	return application.NewTagService(repo)
}

// --- CreateTag ---

func TestCreateTag_ValidIntegerTag_ReturnsID(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := dto.CreateTagRequest{Name: "temperature", Kind: "integer", Value: "42", Quality: "good"}
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

	req := dto.CreateTagRequest{Name: "label", Kind: "string", Value: "hello", Quality: "good"}
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

	req := dto.CreateTagRequest{Name: "enabled", Kind: "boolean", Value: "true", Quality: "good"}
	resp, err := svc.CreateTag(ctx, req)

	if err != nil {
		t.Fatalf("CreateTag returned unexpected error: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Error("expected non-zero ID in response")
	}
}

func TestCreateTag_InvalidKind_ReturnsError(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := dto.CreateTagRequest{Name: "temperature", Kind: "unknown", Value: "42", Quality: "good"}
	_, err := svc.CreateTag(ctx, req)

	if err == nil {
		t.Error("expected error for invalid kind, got nil")
	}
}

func TestCreateTag_InvalidQuality_ReturnsError(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := dto.CreateTagRequest{Name: "temperature", Kind: "integer", Value: "42", Quality: "unknown"}
	_, err := svc.CreateTag(ctx, req)

	if err == nil {
		t.Error("expected error for invalid quality, got nil")
	}
}

func TestCreateTag_InvalidValueForKind_ReturnsError(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := dto.CreateTagRequest{Name: "temperature", Kind: "integer", Value: "not-a-number", Quality: "good"}
	_, err := svc.CreateTag(ctx, req)

	if err == nil {
		t.Error("expected error for value incompatible with kind, got nil")
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
	if len(resp.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(resp.Tags))
	}
}

func TestFindAllTags_MultipleTags_ReturnsAll(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req1 := dto.CreateTagRequest{Name: "temperature", Kind: "integer", Value: "10", Quality: "good"}
	req2 := dto.CreateTagRequest{Name: "pressure", Kind: "integer", Value: "20", Quality: "good"}
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
	if len(resp.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(resp.Tags))
	}
}

// --- FindTagByID ---

func TestFindTagByID_ExistingTag_ReturnsTag(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := dto.CreateTagRequest{Name: "humidity", Kind: "integer", Value: "55", Quality: "good"}
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
	if found.Kind != "integer" {
		t.Errorf("Kind: expected 'integer', got %q", found.Kind)
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

	created, err := svc.CreateTag(ctx, dto.CreateTagRequest{Name: "sensor", Kind: "integer", Value: "10", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	tagID := tag.NewTagID(tag.TagIDWithUUID(created.ID))
	resp, err := svc.DeleteTagByID(ctx, tagID)

	if err != nil {
		t.Fatalf("DeleteTagByID returned unexpected error: %v", err)
	}
	if resp.Tag.ID != created.ID {
		t.Errorf("deleted tag ID: expected %s, got %s", created.ID, resp.Tag.ID)
	}
	if resp.Tag.Name != "sensor" {
		t.Errorf("deleted tag Name: expected 'sensor', got %q", resp.Tag.Name)
	}
}

func TestDeleteTag_ExistingTag_TagIsRemovedFromDB(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, dto.CreateTagRequest{Name: "valve", Kind: "boolean", Value: "true", Quality: "good"})
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

	created, err := svc.CreateTag(ctx, dto.CreateTagRequest{Name: "pressure", Kind: "integer", Value: "100", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	resp, err := svc.SetTagValueByID(ctx, dto.UpdateTagRequest{ID: created.ID, Value: "200", Quality: "good"})

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

	created, err := svc.CreateTag(ctx, dto.CreateTagRequest{Name: "flow", Kind: "integer", Value: "0", Quality: "bad"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	tagID := tag.NewTagID(tag.TagIDWithUUID(created.ID))
	if _, err = svc.SetTagValueByID(ctx, dto.UpdateTagRequest{ID: created.ID, Value: "42", Quality: "good"}); err != nil {
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

	_, err := svc.SetTagValueByID(ctx, dto.UpdateTagRequest{ID: uuid.New(), Value: "1", Quality: "good"})

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

	created, err := svc.CreateTag(ctx, dto.CreateTagRequest{Name: "temp", Kind: "integer", Value: "10", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	_, err = svc.SetTagValueByID(ctx, dto.UpdateTagRequest{ID: created.ID, Value: "20", Quality: "unknown"})

	if err == nil {
		t.Error("expected error for invalid quality, got nil")
	}
}

func TestSetTagValueByID_InvalidValueForKind_ReturnsError(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, dto.CreateTagRequest{Name: "counter", Kind: "integer", Value: "0", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	_, err = svc.SetTagValueByID(ctx, dto.UpdateTagRequest{ID: created.ID, Value: "not-a-number", Quality: "good"})

	if err == nil {
		t.Error("expected error for value incompatible with kind, got nil")
	}
}
