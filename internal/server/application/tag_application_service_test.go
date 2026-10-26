package application_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/migrations"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
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

	if err := migrations.Up(ctx, db); err != nil {
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

// cleanTags removes the Tag definitions and the Tag states.
func cleanTags(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM runtime.tags; DELETE FROM engineering.tags"); err != nil {
		t.Fatalf("cleanTags: %v", err)
	}
}

func newService() application.TagService {
	return application.NewTagService(contentOver(newScopes()), newRuntimeNode())
}

func newServiceWithBus(t *testing.T) (application.TagService, eventbus.EventBus) {
	bus, onCommit := deliveredOnCommit(t)
	return application.NewTagService(contentOver(newScopes(onCommit)), newRuntimeNode()), bus
}

func mustNewTagID() uuid.UUID {
	return uuid.New()
}

// --- CreateTag ---

func TestCreateTag_ValidIntegerTag_ReturnsID(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	req := appdto.CreateTagInput{Name: "temperature", Type: "integer"}
	resp, err := svc.CreateTag(ctx, mainProject, req)

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

	req := appdto.CreateTagInput{Name: "label", Type: "string"}
	resp, err := svc.CreateTag(ctx, mainProject, req)

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

	req := appdto.CreateTagInput{Name: "enabled", Type: "boolean"}
	resp, err := svc.CreateTag(ctx, mainProject, req)

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

	req := appdto.CreateTagInput{Name: "temperature", Type: "unknown"}
	_, err := svc.CreateTag(ctx, mainProject, req)

	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid type, got: %v", err)
	}
}

// --- FindAllTags ---

func TestFindAllTags_EmptyDB_ReturnsEmptyList(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	resp, err := svc.FindAllTags(ctx, mainProject)

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

	req1 := appdto.CreateTagInput{Name: "temperature", Type: "integer"}
	req2 := appdto.CreateTagInput{Name: "pressure", Type: "integer"}
	if _, err := svc.CreateTag(ctx, mainProject, req1); err != nil {
		t.Fatalf("CreateTag(temperature): %v", err)
	}
	if _, err := svc.CreateTag(ctx, mainProject, req2); err != nil {
		t.Fatalf("CreateTag(pressure): %v", err)
	}

	resp, err := svc.FindAllTags(ctx, mainProject)

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

	req := appdto.CreateTagInput{Name: "humidity", Type: "integer"}
	createResp, err := svc.CreateTag(ctx, mainProject, req)
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	found, err := svc.FindTagByID(ctx, mainProject, createResp.ID)

	if err != nil {
		t.Fatalf("FindTagByID returned unexpected error: %v", err)
	}
	if found.Name != "humidity" {
		t.Errorf("Name: expected 'humidity', got %q", found.Name)
	}
	if found.Type != "integer" {
		t.Errorf("Type:expected 'integer', got %q", found.Type)
	}
	if found.Value != "0" || found.Quality != "bad" || found.Seq != 0 {
		t.Errorf("state: expected the initial 0, bad, no seq, got %q, %q, %d", found.Value, found.Quality, found.Seq)
	}
}

func TestFindTagByID_NotFound_ReturnsWrappedErrTagNotFound(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	_, err := svc.FindTagByID(ctx, mainProject, mustNewTagID())

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

	created, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "sensor", Type: "integer"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	resp, err := svc.DeleteTagByID(ctx, mainProject, created.ID)

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

	created, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "valve", Type: "boolean"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	if _, err = svc.DeleteTagByID(ctx, mainProject, created.ID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}

	_, err = svc.FindTagByID(ctx, mainProject, created.ID)
	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected ErrTagNotFound after delete, got: %v", err)
	}
}

func TestDeleteTag_NotFound_ReturnsWrappedErrTagNotFound(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	_, err := svc.DeleteTagByID(ctx, mainProject, mustNewTagID())

	if err == nil {
		t.Fatal("expected error for non-existent tag, got nil")
	}
	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected wrapped ErrTagNotFound, got: %v", err)
	}
}

// --- States from the Runtime node ---

func TestCreateTag_StateIsInitialAndAcceptsValuesAtOnce(t *testing.T) {
	cleanTags(t)
	svc, states := newServiceWithStates()
	ctx := context.Background()

	for tagType, zero := range map[string]string{"integer": "0", "boolean": "false", "string": ""} {
		created, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "t_" + tagType, Type: tagType})
		if err != nil {
			t.Fatalf("CreateTag(%s): %v", tagType, err)
		}
		if created.Value != zero || created.Quality != "bad" || created.Seq != 0 || created.Version != 1 {
			t.Errorf("%s: created %+v, want the zero value %q, bad, no seq, version 1", tagType, created, zero)
		}
		// No policy runs here: the creation gave the Runtime node the
		// definition itself.
		if _, err := states.SetTagValue(ctx, mainProject, runtimeapp.SetTagValueInput{TagID: created.ID, Value: zero, Quality: "good"}); err != nil {
			t.Errorf("%s: a value written right after the creation: %v", tagType, err)
		}
	}
}

func TestFindTags_CarryTheWrittenState(t *testing.T) {
	cleanTags(t)
	svc, states := newServiceWithStates()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "flow", Type: "integer"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	written, err := states.SetTagValue(ctx, mainProject, runtimeapp.SetTagValueInput{TagID: created.ID, Value: "42", Quality: "uncertain"})
	if err != nil {
		t.Fatalf("SetTagValue: %v", err)
	}

	byID, err := svc.FindTagByID(ctx, mainProject, created.ID)
	if err != nil {
		t.Fatalf("FindTagByID: %v", err)
	}
	all, err := svc.FindAllTags(ctx, mainProject)
	if err != nil || len(all) != 1 {
		t.Fatalf("FindAllTags: %v, %v", all, err)
	}
	byName, err := svc.FindTagByName(ctx, mainProject, "flow")
	if err != nil {
		t.Fatalf("FindTagByName: %v", err)
	}
	for how, got := range map[string]appdto.Tag{"by id": byID, "all": all[0], "by name": byName} {
		if got.Value != "42" || got.Quality != "uncertain" || got.Seq != written.Seq || got.Version != created.Version {
			t.Errorf("%s: %+v, want 42, uncertain, seq %d, the definition's version %d", how, got, written.Seq, created.Version)
		}
	}
}

// TestFindTags_WithoutAStateShowTheInitialOne: a definition the Runtime node
// has not been given yet (its upsert failed, the policy has not run) reads
// with its initial state.
func TestFindTags_WithoutAStateShowTheInitialOne(t *testing.T) {
	cleanTags(t)
	svc := application.NewTagService(contentOver(newScopes()), failingRuntimeNode{RuntimeNode: newRuntimeNode()})
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "flag", Type: "boolean"})
	if err != nil {
		t.Fatalf("CreateTag with the Runtime node failing: %v", err)
	}
	found, err := svc.FindTagByID(ctx, mainProject, created.ID)
	if err != nil {
		t.Fatalf("FindTagByID: %v", err)
	}
	if found.Value != "false" || found.Quality != "bad" || found.Seq != 0 {
		t.Errorf("found %+v, want the initial false, bad", found)
	}
}

func TestDeleteTag_RemovesItsStateAtOnce(t *testing.T) {
	cleanTags(t)
	svc, states := newServiceWithStates()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "valve", Type: "boolean"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := states.SetTagValue(ctx, mainProject, runtimeapp.SetTagValueInput{TagID: created.ID, Value: "true", Quality: "good"}); err != nil {
		t.Fatalf("SetTagValue: %v", err)
	}

	deleted, err := svc.DeleteTagByID(ctx, mainProject, created.ID)
	if err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}
	if deleted.Value != "true" || deleted.Quality != "good" {
		t.Errorf("deleted %+v, want its last state true, good", deleted)
	}
	_, err = states.SetTagValue(ctx, mainProject, runtimeapp.SetTagValueInput{TagID: created.ID, Value: "false", Quality: "good"})
	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("a value written after the deletion: %v, want ErrTagNotFound", err)
	}
}

// --- Events ---

func TestCreateTag_Success_PublishesTagCreatedEvent(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus(t)
	ctx := context.Background()

	var received []event.Event
	bus.Subscribe(event.EventTypeTagCreated, func(e event.Event) {
		received = append(received, e)
	})

	resp, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "sensor", Type: "integer"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	tagEvent, ok := received[0].(tag.TagEvent)
	if !ok {
		t.Fatal("expected event to implement TagEvent")
	}
	if tagEvent.TagID().UUID() != resp.ID {
		t.Errorf("event TagID: expected %s, got %s", resp.ID, tagEvent.TagID().UUID())
	}
}

func TestCreateTag_InvalidRequest_NoEventPublished(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus(t)
	ctx := context.Background()

	var received []event.Event
	bus.Subscribe(event.EventTypeTagCreated, func(e event.Event) {
		received = append(received, e)
	})

	_, _ = svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "sensor", Type: "unknown"})

	if len(received) != 0 {
		t.Errorf("expected no events on error, got %d", len(received))
	}
}

func TestDeleteTagByID_Success_PublishesTagDeletedEvent(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus(t)
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "valve", Type: "boolean"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	var received []event.Event
	bus.Subscribe(event.EventTypeTagDeleted, func(e event.Event) {
		received = append(received, e)
	})

	if _, err = svc.DeleteTagByID(ctx, mainProject, created.ID); err != nil {
		t.Fatalf("DeleteTagByID: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	tagEvent, ok := received[0].(tag.TagEvent)
	if !ok {
		t.Fatal("expected event to implement TagEvent")
	}
	if tagEvent.TagID().UUID() != created.ID {
		t.Errorf("event TagID: expected %s, got %s", created.ID, tagEvent.TagID().UUID())
	}
}

func TestDeleteTagByID_NotFound_NoEventPublished(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus(t)
	ctx := context.Background()

	var received []event.Event
	bus.Subscribe(event.EventTypeTagDeleted, func(e event.Event) {
		received = append(received, e)
	})

	_, _ = svc.DeleteTagByID(ctx, mainProject, mustNewTagID())

	if len(received) != 0 {
		t.Errorf("expected no events on error, got %d", len(received))
	}
}

// --- FindTagByName / unique name ---

func TestFindTagByName_ExistingTag_ReturnsTag(t *testing.T) {
	cleanTags(t)
	svc := newService()
	ctx := context.Background()

	created, err := svc.CreateTag(ctx, mainProject, appdto.CreateTagInput{Name: "temperature", Type: "integer"})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	found, err := svc.FindTagByName(ctx, mainProject, "temperature")

	if err != nil {
		t.Fatalf("FindTagByName returned unexpected error: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("id: expected %v, got %v", created.ID, found.ID)
	}
}

func TestFindTagByName_NotFound_ReturnsWrappedErrTagNotFound(t *testing.T) {
	cleanTags(t)
	svc := newService()

	_, err := svc.FindTagByName(context.Background(), mainProject, "missing")

	if !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("expected wrapped ErrTagNotFound, got: %v", err)
	}
}

func TestCreateTag_DuplicateName_ReturnsWrappedErrTagNameTaken(t *testing.T) {
	cleanTags(t)
	svc, bus := newServiceWithBus(t)
	ctx := context.Background()

	req := appdto.CreateTagInput{Name: "temperature", Type: "integer"}
	if _, err := svc.CreateTag(ctx, mainProject, req); err != nil {
		t.Fatalf("first CreateTag failed: %v", err)
	}
	var published []event.Event
	bus.Subscribe(event.EventTypeTagCreated, func(e event.Event) { published = append(published, e) })

	_, err := svc.CreateTag(ctx, mainProject, req)

	if !errors.Is(err, tag.ErrTagNameTaken) {
		t.Errorf("expected wrapped ErrTagNameTaken, got: %v", err)
	}
	if len(published) != 0 {
		t.Errorf("expected no TagCreated event, got %d", len(published))
	}
}

// --- FindTagsByNamePattern / FindTagsByNameRegex ---

func createTags(t *testing.T, svc application.TagService, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := svc.CreateTag(context.Background(), mainProject, appdto.CreateTagInput{Name: name, Type: "integer"}); err != nil {
			t.Fatalf("CreateTag(%q): %v", name, err)
		}
	}
}

func tagNames(tags []appdto.Tag) map[string]bool {
	names := make(map[string]bool, len(tags))
	for _, tg := range tags {
		names[tg.Name] = true
	}
	return names
}

func TestFindTagsByNamePattern_ReturnsMatchingTags(t *testing.T) {
	cleanTags(t)
	svc := newService()
	createTags(t, svc, "pump1.speed", "pump2.speed", "pump12.speed", "valve1.state")

	got, err := svc.FindTagsByNamePattern(context.Background(), mainProject, "pump?.speed")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := tagNames(got)
	if len(names) != 2 || !names["pump1.speed"] || !names["pump2.speed"] {
		t.Errorf("got %v, want pump1.speed and pump2.speed", names)
	}
}

func TestFindTagsByNameRegex_ReturnsMatchingTags(t *testing.T) {
	cleanTags(t)
	svc := newService()
	createTags(t, svc, "pump1.speed", "pump12.speed", "valve1.state")

	got, err := svc.FindTagsByNameRegex(context.Background(), mainProject, `^pump\d+\.`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := tagNames(got)
	if len(names) != 2 || !names["pump1.speed"] || !names["pump12.speed"] {
		t.Errorf("got %v, want pump1.speed and pump12.speed", names)
	}
}

func TestFindTagsByNameRegex_Invalid_ReturnsErrInvalidTagNameMatcher(t *testing.T) {
	svc := newService()

	_, err := svc.FindTagsByNameRegex(context.Background(), mainProject, `pump(`)

	if !errors.Is(err, tag.ErrInvalidTagNameMatcher) {
		t.Errorf("expected ErrInvalidTagNameMatcher, got %v", err)
	}
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}
