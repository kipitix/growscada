package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func cleanOutbox(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM outbox"); err != nil {
		t.Fatalf("cleanOutbox: %v", err)
	}
}

// outboxTypes returns the types of the events in the outbox, oldest first.
func outboxTypes(t *testing.T) []string {
	t.Helper()
	rows, err := testDB.QueryContext(context.Background(), "SELECT type FROM outbox ORDER BY seq")
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		types = append(types, s)
	}
	return types
}

// recorder collects the events a Dispatcher delivers.
type recorder struct {
	mu     sync.Mutex
	events []event.Event
}

func (r *recorder) deliver(e event.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	for i, e := range r.events {
		out[i] = e.Type().String()
	}
	return out
}

func createTag(t *testing.T, repo tag.TagRepository, name string) tag.Tag {
	t.Helper()
	tmpl := makeTag(t, name, repo)
	created, err := tag.CreateTag(tmpl.ID(), tmpl.Name(), tmpl.Type(), tmpl.Value(), tmpl.Quality())
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	return created
}

func TestSave_WritesRecordedEventsToOutbox(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	ctx := context.Background()

	saved := mustSaveTag(t, repo, createTag(t, repo, "level"))
	if len(saved.PendingEvents()) != 0 {
		t.Errorf("the saved tag must record nothing, got %v", saved.PendingEvents())
	}
	if err := saved.SetValue(int64(5), tag.TagQualityGood); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	saved = mustSaveTag(t, repo, saved)
	saved.Delete()
	if err := repo.Delete(ctx, saved); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	want := []string{"tag_created", "tag_updated", "tag_deleted"}
	if got := outboxTypes(t); !slices.Equal(got, want) {
		t.Errorf("outbox %v, want %v", got, want)
	}
}

func TestSave_RolledBack_LeavesNothingInOutboxAndDeliversNothing(t *testing.T) {
	cleanTags(t)
	cleanScenes(t)
	cleanOutbox(t)
	ctx := context.Background()
	tagRepo := repositories.NewTagRepositoryPostgres(testDB)
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB)
	mustSaveTag(t, tagRepo, makeTag(t, "taken", tagRepo))

	// A taken name fails the insert itself.
	if _, err := tagRepo.Save(ctx, createTag(t, tagRepo, "taken")); !errors.Is(err, tag.ErrTagNameTaken) {
		t.Fatalf("expected ErrTagNameTaken, got %v", err)
	}
	// A widget of a missing type fails after the scene row was written.
	stored := mustSaveScene(t, sceneRepo, "scene-1")
	w := makeWidget(t, sceneRepo, "w1")
	wt, err := repositories.NewWidgetTypeRepositoryPostgres(testDB).FindByID(ctx, w.TypeID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, "DELETE FROM widget_types WHERE id = $1", w.TypeID().UUID()); err != nil {
		t.Fatalf("delete widget type: %v", err)
	}
	changed, err := stored.AddWidget(stored.Version(), w, wt)
	if err != nil {
		t.Fatalf("AddWidget: %v", err)
	}
	if _, err := sceneRepo.Save(ctx, changed); !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Fatalf("expected ErrWidgetTypeNotFound, got %v", err)
	}

	if got := outboxTypes(t); len(got) != 0 {
		t.Errorf("expected an empty outbox, got %v", got)
	}
	var delivered recorder
	n, err := outbox.NewDispatcher(testDB, delivered.deliver).DrainOnce(ctx)
	if err != nil || n != 0 || len(delivered.types()) != 0 {
		t.Errorf("expected nothing delivered, got %d %v (err %v)", n, delivered.types(), err)
	}
	if found := mustFindByID(t, sceneRepo, stored.ID()); found.Version() != stored.Version() {
		t.Errorf("rolled back scene changed version to %s", found.Version())
	}
}

func TestDispatcher_DrainOnce_DeliversInCommitOrderAndEmptiesOutbox(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	ctx := context.Background()
	repo := repositories.NewTagRepositoryPostgres(testDB)

	a := mustSaveTag(t, repo, createTag(t, repo, "a"))
	b := mustSaveTag(t, repo, createTag(t, repo, "b"))
	if err := a.SetValue(int64(1), tag.TagQualityGood); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	mustSaveTag(t, repo, a)
	b.Delete()
	if err := repo.Delete(ctx, b); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var delivered recorder
	d := outbox.NewDispatcher(testDB, delivered.deliver)
	n, err := d.DrainOnce(ctx)
	if err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}

	want := []string{"tag_created", "tag_created", "tag_updated", "tag_deleted"}
	if got := delivered.types(); n != 4 || !slices.Equal(got, want) {
		t.Errorf("delivered %d %v, want %v", n, got, want)
	}
	created := delivered.events[0].(tag.TagCreatedEvent)
	if created.TagID() != a.ID() {
		t.Errorf("first event is about %s, want %s", created.TagID(), a.ID())
	}
	updated := delivered.events[2].(tag.TagUpdatedEvent)
	if updated.Tag().Value().String() != "1" || updated.Tag().Version().Number() != 2 {
		t.Errorf("tag_updated carries %s, want value 1 at version 2", updated.Tag())
	}
	if got := outboxTypes(t); len(got) != 0 {
		t.Errorf("expected an empty outbox after delivery, got %v", got)
	}
	if n, _ := d.DrainOnce(ctx); n != 0 {
		t.Errorf("a delivered event was delivered again: %d", n)
	}
}

func TestDispatcher_Run_DeliversOnNotify(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var delivered recorder
	// A poll interval long enough that only Notify can explain a delivery.
	d := outbox.NewDispatcher(testDB, delivered.deliver, outbox.WithPollInterval(time.Hour))
	done := make(chan struct{})
	go func() { d.Run(ctx); close(done) }()

	repo := repositories.NewTagRepositoryPostgres(testDB, repositories.NotifyOnCommit(d.Notify))
	mustSaveTag(t, repo, createTag(t, repo, "notified"))

	deadline := time.Now().Add(5 * time.Second)
	for len(delivered.types()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := delivered.types(); !slices.Equal(got, []string{"tag_created"}) {
		t.Errorf("delivered %v, want [tag_created]", got)
	}
	cancel()
	<-done
}

func TestDispatcher_UndecodableRow_IsDroppedAndDoesNotBlock(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, `INSERT INTO outbox (type, payload) VALUES ('no_such_event', '{}')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	repo := repositories.NewTagRepositoryPostgres(testDB)
	mustSaveTag(t, repo, createTag(t, repo, "after"))

	var delivered recorder
	if _, err := outbox.NewDispatcher(testDB, delivered.deliver).DrainOnce(ctx); err != nil {
		t.Fatalf("DrainOnce: %v", err)
	}
	if got := delivered.types(); !slices.Equal(got, []string{"tag_created"}) {
		t.Errorf("delivered %v, want [tag_created]", got)
	}
	if got := outboxTypes(t); len(got) != 0 {
		t.Errorf("expected an empty outbox, got %v", got)
	}
}

// beginWithID begins a transaction and makes it take its id, as its first
// write would.
func beginWithID(t *testing.T) *sql.Tx {
	t.Helper()
	tx, err := testDB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	if _, err := tx.Exec(`SELECT pg_current_xact_id()`); err != nil {
		t.Fatalf("take transaction id: %v", err)
	}
	return tx
}

func appendDeleted(t *testing.T, ctx context.Context, tx *sql.Tx, tagID id.ID[tag.Tag]) {
	t.Helper()
	if err := outbox.Append(ctx, tx, []event.Event{tag.NewTagDeletedEvent(tagID)}); err != nil {
		t.Fatalf("Append: %v", err)
	}
}

func TestAppend_WritersDoNotWaitForEachOther(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()

	open := beginWithID(t)
	appendDeleted(t, ctx, open, id.NewID[tag.Tag]())

	// Another writer appends and commits while the first one is still open.
	quick, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := testDB.BeginTx(quick, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	appendDeleted(t, quick, tx, id.NewID[tag.Tag]())
	if err := tx.Commit(); err != nil {
		t.Fatalf("a writer waited for another one's commit: %v", err)
	}
}

func TestDispatcher_HoldsBackRowsUntilOlderTransactionsEnd(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	first, second := id.NewID[tag.Tag](), id.NewID[tag.Tag]()

	older := beginWithID(t)
	younger := beginWithID(t)
	appendDeleted(t, ctx, younger, second)
	if err := younger.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var delivered recorder
	d := outbox.NewDispatcher(testDB, delivered.deliver)
	if n, err := d.DrainOnce(ctx); err != nil || n != 0 {
		t.Fatalf("delivered %d (err %v) while an older transaction runs, want 0", n, err)
	}

	appendDeleted(t, ctx, older, first)
	if err := older.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if n, err := d.DrainOnce(ctx); err != nil || n != 2 {
		t.Fatalf("delivered %d (err %v), want 2", n, err)
	}
	got := []id.ID[tag.Tag]{
		delivered.events[0].(tag.TagDeletedEvent).TagID(),
		delivered.events[1].(tag.TagDeletedEvent).TagID(),
	}
	if got[0] != first || got[1] != second {
		t.Errorf("delivered %v, want the older transaction's event first: [%s %s]", got, first, second)
	}
}

func TestDispatcher_Run_DeliversHeldBackRowsWhenOlderTransactionRollsBack(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var delivered recorder
	// A poll interval long enough that only Notify and the held back retry
	// can explain a delivery.
	d := outbox.NewDispatcher(testDB, delivered.deliver, outbox.WithPollInterval(time.Hour))
	done := make(chan struct{})
	go func() { d.Run(ctx); close(done) }()

	older := beginWithID(t)
	repo := repositories.NewTagRepositoryPostgres(testDB, repositories.NotifyOnCommit(d.Notify))
	mustSaveTag(t, repo, createTag(t, repo, "held"))
	// The rollback sends no Notify.
	if err := older.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for len(delivered.types()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := delivered.types(); !slices.Equal(got, []string{"tag_created"}) {
		t.Errorf("delivered %v, want [tag_created]", got)
	}
	cancel()
	<-done
}

func TestDispatcher_DrainOnce_StoppedMidBatch_DeletesDeliveredRowAndStops(t *testing.T) {
	cleanTags(t)
	cleanOutbox(t)
	repo := repositories.NewTagRepositoryPostgres(testDB)
	for _, name := range []string{"a", "b", "c"} {
		mustSaveTag(t, repo, createTag(t, repo, name))
	}

	ctx, cancel := context.WithCancel(context.Background())
	var delivered recorder
	n, err := outbox.NewDispatcher(testDB, func(e event.Event) {
		delivered.deliver(e)
		cancel() // the dispatcher is stopped while it delivers
	}).DrainOnce(ctx)
	if !errors.Is(err, context.Canceled) || n != 1 {
		t.Fatalf("DrainOnce stopped after %d with %v, want 1 and context.Canceled", n, err)
	}
	if got := outboxTypes(t); len(got) != 2 {
		t.Fatalf("outbox %v, want the 2 undelivered rows", got)
	}

	n, err = outbox.NewDispatcher(testDB, delivered.deliver).DrainOnce(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("second DrainOnce delivered %d (err %v), want 2", n, err)
	}
	if got := delivered.types(); len(got) != 3 {
		t.Errorf("delivered %v, want each of the 3 events once", got)
	}
}

func TestDispatcher_DrainOnce_DeliversMoreThanOneBatch(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	const total = 250 // more than two batches of the dispatcher

	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	events := make([]event.Event, total)
	for i := range events {
		events[i] = tag.NewTagDeletedEvent(id.NewID[tag.Tag]())
	}
	if err := outbox.Append(ctx, tx, events); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var delivered recorder
	n, err := outbox.NewDispatcher(testDB, delivered.deliver).DrainOnce(ctx)
	if err != nil || n != total {
		t.Fatalf("delivered %d (err %v), want %d", n, err, total)
	}
	for i, e := range delivered.events {
		if got := e.(tag.TagDeletedEvent).TagID(); got != events[i].(tag.TagDeletedEvent).TagID() {
			t.Fatalf("event %d is about %s, want %s", i, got, events[i].(tag.TagDeletedEvent).TagID())
		}
	}
	if got := outboxTypes(t); len(got) != 0 {
		t.Errorf("expected an empty outbox, got %d rows", len(got))
	}
}
