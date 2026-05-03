package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

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
	migrationsDir := filepath.Join(filepath.Dir(currentFile), "..", "migrations")

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

func makeTag(t *testing.T, name string, repo tag.TagRepository) tag.Tag {
	t.Helper()
	id := repo.NextID()
	tagName, err := tag.NewTagName(name)
	if err != nil {
		t.Fatalf("NewTagName(%q): %v", name, err)
	}
	kind := tag.TagKindInteger
	value, err := kind.NewTagValue(0)
	if err != nil {
		t.Fatalf("NewTagValue: %v", err)
	}
	newTag, err := tag.NewTag(id, tagName, kind, value, tag.TagQualityGood, tag.TagVersionInitial)
	if err != nil {
		t.Fatalf("NewTag: %v", err)
	}
	return newTag
}

func TestNextID_ReturnsUniqueIDs(t *testing.T) {
	repo := repositories.NewTagRepositoryPostgres(testDB)

	id1 := repo.NextID()
	id2 := repo.NextID()

	if id1 == id2 {
		t.Error("NextID should return unique IDs, got duplicates")
	}
}

func TestSave_NewTag_InsertsSuccessfully(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	newTag := makeTag(t, "temperature", repo)

	err := repo.Save(ctx, newTag)

	if err != nil {
		t.Fatalf("Save returned unexpected error: %v", err)
	}
}

func TestSave_DuplicateID_ReturnsError(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	newTag := makeTag(t, "temperature", repo)
	if err := repo.Save(ctx, newTag); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	duplicate, _ := tag.NewTag(newTag.ID(), newTag.Name(), newTag.Kind(), newTag.Value(), newTag.Quality(), tag.TagVersionInitial)
	err := repo.Save(ctx, duplicate)

	if err == nil {
		t.Error("expected error on duplicate insert, got nil")
	}
}

func TestSave_ExistingTag_UpdatesSuccessfully(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	newTag := makeTag(t, "temperature", repo)
	if err := repo.Save(ctx, newTag); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	found, err := repo.FindByID(ctx, newTag.ID())
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if err := found.SetValue(99, tag.TagQualitySimulated); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}
	if err := repo.Save(ctx, found); err != nil {
		t.Fatalf("update Save failed: %v", err)
	}

	updated, err := repo.FindByID(ctx, newTag.ID())
	if err != nil {
		t.Fatalf("FindByID after update failed: %v", err)
	}
	if updated.Value().String() != "99" {
		t.Errorf("value: expected '99', got %q", updated.Value().String())
	}
	if updated.Quality() != tag.TagQualitySimulated {
		t.Errorf("quality: expected simulated, got %v", updated.Quality())
	}
}

func TestSave_StaleVersion_ReturnsError(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	newTag := makeTag(t, "temperature", repo)
	if err := repo.Save(ctx, newTag); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	// version=100 while DB has version=1 → optimistic lock conflict
	staleTag, _ := tag.NewTag(newTag.ID(), newTag.Name(), newTag.Kind(), newTag.Value(), newTag.Quality(), 100)
	err := repo.Save(ctx, staleTag)

	if err == nil {
		t.Error("expected error on stale version update, got nil")
	}
}

func TestFindByID_ExistingTag_ReturnsTag(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	newTag := makeTag(t, "pressure", repo)
	if err := repo.Save(ctx, newTag); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	found, err := repo.FindByID(ctx, newTag.ID())

	if err != nil {
		t.Fatalf("FindByID returned unexpected error: %v", err)
	}
	if found.ID() != newTag.ID() {
		t.Errorf("ID: expected %v, got %v", newTag.ID(), found.ID())
	}
	if found.Name() != newTag.Name() {
		t.Errorf("Name: expected %v, got %v", newTag.Name(), found.Name())
	}
	if found.Kind() != newTag.Kind() {
		t.Errorf("Kind: expected %v, got %v", newTag.Kind(), found.Kind())
	}
	if found.Quality() != newTag.Quality() {
		t.Errorf("Quality: expected %v, got %v", newTag.Quality(), found.Quality())
	}
	if found.Version() != newTag.Version() {
		t.Errorf("Version: expected %d, got %d", newTag.Version(), found.Version())
	}
}

func TestFindByID_NotFound_ReturnsErrTagNotFound(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	nonExistentID := repo.NextID()
	_, err := repo.FindByID(ctx, nonExistentID)

	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected ErrTagNotFound, got %v", err)
	}
}

func TestFindAll_EmptyDB_ReturnsEmptySlice(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	tags, err := repo.FindAll(ctx)

	if err != nil {
		t.Fatalf("FindAll returned unexpected error: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

func TestFindAll_MultipleTags_ReturnsAll(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	tag1 := makeTag(t, "temperature", repo)
	tag2 := makeTag(t, "pressure", repo)
	if err := repo.Save(ctx, tag1); err != nil {
		t.Fatalf("Save tag1 failed: %v", err)
	}
	if err := repo.Save(ctx, tag2); err != nil {
		t.Fatalf("Save tag2 failed: %v", err)
	}

	allTags, err := repo.FindAll(ctx)

	if err != nil {
		t.Fatalf("FindAll returned unexpected error: %v", err)
	}
	if len(allTags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(allTags))
	}
}

// --- Delete ---

func TestDeleteByID_ExistingTag_ReturnsDeletedTag(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	newTag := makeTag(t, "valve", repo)
	if err := repo.Save(ctx, newTag); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	deleted, err := repo.DeleteByID(ctx, newTag.ID())

	if err != nil {
		t.Fatalf("DeleteByID returned unexpected error: %v", err)
	}
	if deleted.ID() != newTag.ID() {
		t.Errorf("ID: expected %v, got %v", newTag.ID(), deleted.ID())
	}
	if deleted.Name() != newTag.Name() {
		t.Errorf("Name: expected %v, got %v", newTag.Name(), deleted.Name())
	}
	if deleted.Kind() != newTag.Kind() {
		t.Errorf("Kind: expected %v, got %v", newTag.Kind(), deleted.Kind())
	}
	if deleted.Value().String() != "0" {
		t.Errorf("Value: expected '0', got %q", deleted.Value().String())
	}
	if deleted.Quality() != tag.TagQualityGood {
		t.Errorf("Quality: expected good, got %v", deleted.Quality())
	}
	if deleted.Version() != 1 {
		t.Errorf("Version: expected 1, got %d", deleted.Version())
	}
}

func TestDeleteByID_ExistingTag_TagIsRemovedFromDB(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	newTag := makeTag(t, "pump", repo)
	if err := repo.Save(ctx, newTag); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	deleted, err := repo.DeleteByID(ctx, newTag.ID())
	if err != nil {
		t.Fatalf("DeleteByID failed: %v", err)
	}
	if deleted.ID() != newTag.ID() {
		t.Errorf("ID: expected %v, got %v", newTag.ID(), deleted.ID())
	}
	if deleted.Name() != newTag.Name() {
		t.Errorf("Name: expected %v, got %v", newTag.Name(), deleted.Name())
	}

	_, err = repo.FindByID(ctx, newTag.ID())

	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected ErrTagNotFound after delete, got %v", err)
	}
}

func TestDeleteByID_ExistingTag_OtherTagsAreUnaffected(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	tag1 := makeTag(t, "temperature", repo)
	tag2 := makeTag(t, "pressure", repo)
	if err := repo.Save(ctx, tag1); err != nil {
		t.Fatalf("Save tag1 failed: %v", err)
	}
	if err := repo.Save(ctx, tag2); err != nil {
		t.Fatalf("Save tag2 failed: %v", err)
	}

	if _, err := repo.DeleteByID(ctx, tag1.ID()); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := repo.FindByID(ctx, tag2.ID())
	if err != nil {
		t.Errorf("tag2 should still exist after deleting tag1, got error: %v", err)
	}
}

func TestDeleteByID_NotFound_ReturnsErrTagNotFound(t *testing.T) {
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	nonExistentID := repo.NextID()
	_, err := repo.DeleteByID(ctx, nonExistentID)

	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected ErrTagNotFound, got %v", err)
	}
}
