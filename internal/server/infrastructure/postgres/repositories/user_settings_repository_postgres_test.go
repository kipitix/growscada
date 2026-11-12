package repositories_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/settings"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

var (
	firstLibrary  = uuid.MustParse("d0000000-0000-4000-8000-000000000001")
	secondLibrary = uuid.MustParse("d0000000-0000-4000-8000-000000000002")
)

// cleanUserSettings removes every per-user setting written by a test.
func cleanUserSettings(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), `DELETE FROM user_settings`); err != nil {
		t.Fatalf("cleanUserSettings: %v", err)
	}
}

// TestUserSettings_InsertFindAndUpdate covers the life of one setting (task
// 81): nothing stored, the first write inserts at Version 1, and every later
// write raises the Version (ADR 0024).
func TestUserSettings_InsertFindAndUpdate(t *testing.T) {
	cleanUserSettings(t)
	repo := repositories.NewUserSettingsRepositoryPostgres(testDB)
	ctx := context.Background()
	own, org := settings.SingleUser(), soleOrganization(t)

	if _, ok, err := repo.FindLibraryOrder(ctx, own, org); err != nil || ok {
		t.Fatalf("Find of settings nobody wrote: %t, %v", ok, err)
	}

	empty := settings.Empty(own, org)
	saved, err := repo.SaveLibraryOrder(ctx, empty.WithLibraryOrder(settings.NewOrder([]uuid.UUID{secondLibrary, firstLibrary})))
	if err != nil {
		t.Fatalf("Save the first order: %v", err)
	}
	if saved.Version().Number() != 1 {
		t.Errorf("the first write stored version %d, want 1", saved.Version().Number())
	}

	found, ok, err := repo.FindLibraryOrder(ctx, own, org)
	if err != nil || !ok {
		t.Fatalf("Find after the insert: %t, %v", ok, err)
	}
	if found.Version().Number() != 1 {
		t.Errorf("the stored version: %d, want 1", found.Version().Number())
	}
	if got := found.LibraryOrder().IDs(); len(got) != 2 || got[0] != secondLibrary || got[1] != firstLibrary {
		t.Errorf("the stored order: %v", got)
	}
	if found.Owner() != own || found.OrganizationID() != org {
		t.Errorf("the stored settings belong to %s in %s", found.Owner(), found.OrganizationID())
	}

	// A find of another owner finds nothing: the setting is one person's.
	other := id.NewID(id.IDWithUUID[settings.User](uuid.New()))
	if _, ok, err := repo.FindLibraryOrder(ctx, other, org); err != nil || ok {
		t.Errorf("Find of another owner: %t, %v", ok, err)
	}
	// And a write of another owner leaves this one's row as it is.
	if _, err := repo.SaveLibraryOrder(ctx, settings.Empty(other, org).
		WithLibraryOrder(settings.NewOrder([]uuid.UUID{firstLibrary}))); err != nil {
		t.Fatalf("Save for another owner: %v", err)
	}
	if mine, ok, err := repo.FindLibraryOrder(ctx, own, org); err != nil || !ok {
		t.Fatalf("Find after another owner's write: %t, %v", ok, err)
	} else if got := mine.LibraryOrder().IDs(); len(got) != 2 || got[0] != secondLibrary || got[1] != firstLibrary {
		t.Errorf("another owner's write changed the order: %v", got)
	}

	updated, err := repo.SaveLibraryOrder(ctx, found.WithLibraryOrder(settings.NewOrder([]uuid.UUID{firstLibrary})))
	if err != nil {
		t.Fatalf("Save the second order: %v", err)
	}
	if updated.Version().Number() != 2 {
		t.Errorf("the second write stored version %d, want 2", updated.Version().Number())
	}
	found, ok, err = repo.FindLibraryOrder(ctx, own, org)
	if err != nil || !ok {
		t.Fatalf("Find after the update: %t, %v", ok, err)
	}
	if got := found.LibraryOrder().IDs(); len(got) != 1 || got[0] != firstLibrary {
		t.Errorf("the updated order: %v", got)
	}
}

// TestUserSettings_StaleVersionIsAConflict: two writers that read one Version
// do not overwrite each other silently; the first insert of a row another
// write created is a conflict too (task 81).
func TestUserSettings_StaleVersionIsAConflict(t *testing.T) {
	cleanUserSettings(t)
	repo := repositories.NewUserSettingsRepositoryPostgres(testDB)
	ctx := context.Background()
	own, org := settings.SingleUser(), soleOrganization(t)
	empty := settings.Empty(own, org)

	first, err := repo.SaveLibraryOrder(ctx, empty.WithLibraryOrder(settings.NewOrder([]uuid.UUID{firstLibrary})))
	if err != nil {
		t.Fatalf("Save the first order: %v", err)
	}
	if _, err := repo.SaveLibraryOrder(ctx, empty.WithLibraryOrder(settings.NewOrder([]uuid.UUID{secondLibrary}))); !errors.Is(err, settings.ErrSettingsConflict) {
		t.Errorf("a second insert of one setting: %v, want ErrSettingsConflict", err)
	}

	// Two writers that read Version 1: the second write is refused, and the
	// order the first one stored stays.
	if _, err := repo.SaveLibraryOrder(ctx, first.WithLibraryOrder(settings.NewOrder([]uuid.UUID{secondLibrary}))); err != nil {
		t.Fatalf("Save at the version read: %v", err)
	}
	if _, err := repo.SaveLibraryOrder(ctx, first.WithLibraryOrder(settings.NewOrder([]uuid.UUID{firstLibrary}))); !errors.Is(err, settings.ErrSettingsConflict) {
		t.Errorf("a write at a stale version: %v, want ErrSettingsConflict", err)
	}
	found, ok, err := repo.FindLibraryOrder(ctx, own, org)
	if err != nil || !ok {
		t.Fatalf("Find after the conflicts: %t, %v", ok, err)
	}
	if got := found.LibraryOrder().IDs(); len(got) != 1 || got[0] != secondLibrary {
		t.Errorf("a refused write changed the order: %v", got)
	}
}

// TestUserSettings_OrganizationsAreApart: the same person keeps a different
// order per Organization, so a new tenant starts from an empty one (task 81).
func TestUserSettings_OrganizationsAreApart(t *testing.T) {
	cleanUserSettings(t)
	repo := repositories.NewUserSettingsRepositoryPostgres(testDB)
	ctx := context.Background()
	own := settings.SingleUser()
	firstOrg := soleOrganization(t)

	var secondOrg uuid.UUID
	if err := testDB.QueryRowContext(ctx,
		`INSERT INTO organizations (id, name) VALUES ($1, 'Second') RETURNING id`,
		uuid.New(),
	).Scan(&secondOrg); err != nil {
		t.Fatalf("insert the second organization: %v", err)
	}
	otherOrg := id.NewID(id.IDWithUUID[organization.Organization](secondOrg))
	// The package's other tests read the sole Organization the migrations
	// create: the second one goes away with the settings that reference it.
	t.Cleanup(func() {
		if _, err := testDB.ExecContext(context.Background(),
			`DELETE FROM user_settings WHERE organization_id = $1`, secondOrg); err != nil {
			t.Errorf("clean the second organization's settings: %v", err)
		}
		if _, err := testDB.ExecContext(context.Background(),
			`DELETE FROM organizations WHERE id = $1`, secondOrg); err != nil {
			t.Errorf("clean the second organization: %v", err)
		}
	})

	if _, err := repo.SaveLibraryOrder(ctx, settings.Empty(own, firstOrg).
		WithLibraryOrder(settings.NewOrder([]uuid.UUID{firstLibrary}))); err != nil {
		t.Fatalf("Save in the first organization: %v", err)
	}
	if _, ok, err := repo.FindLibraryOrder(ctx, own, otherOrg); err != nil || ok {
		t.Errorf("the second organization has the first one's order: %t, %v", ok, err)
	}
	if _, err := repo.SaveLibraryOrder(ctx, settings.Empty(own, otherOrg).
		WithLibraryOrder(settings.NewOrder([]uuid.UUID{secondLibrary}))); err != nil {
		t.Fatalf("Save in the second organization: %v", err)
	}

	first, ok, err := repo.FindLibraryOrder(ctx, own, firstOrg)
	if err != nil || !ok {
		t.Fatalf("Find in the first organization: %t, %v", ok, err)
	}
	second, ok, err := repo.FindLibraryOrder(ctx, own, otherOrg)
	if err != nil || !ok {
		t.Fatalf("Find in the second organization: %t, %v", ok, err)
	}
	if got := first.LibraryOrder().IDs(); len(got) != 1 || got[0] != firstLibrary {
		t.Errorf("the first organization's order: %v", got)
	}
	if got := second.LibraryOrder().IDs(); len(got) != 1 || got[0] != secondLibrary {
		t.Errorf("the second organization's order: %v", got)
	}
}

// raceWrites runs every write at once, from one start, and returns their
// errors in order.
func raceWrites(writes ...func() error) []error {
	errs := make([]error, len(writes))
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(len(writes))
	for i, write := range writes {
		go func(i int, write func() error) {
			defer wg.Done()
			<-start
			errs[i] = write()
		}(i, write)
	}
	close(start)
	wg.Wait()
	return errs
}

// TestUserSettings_RacingWrites_OneWins: two writers that read one Version do
// not overwrite each other. The insert and the update are each one statement
// whose compare-and-swap lets a single writer through and answers the other
// ErrSettingsConflict (task 81, ADR 0024).
func TestUserSettings_RacingWrites_OneWins(t *testing.T) {
	cleanUserSettings(t)
	repo := repositories.NewUserSettingsRepositoryPostgres(testDB)
	ctx := context.Background()
	own, org := settings.SingleUser(), soleOrganization(t)
	empty := settings.Empty(own, org)

	checkOneWins := func(t *testing.T, errs []error) {
		t.Helper()
		var won, conflicted int
		for _, err := range errs {
			switch {
			case err == nil:
				won++
			case errors.Is(err, settings.ErrSettingsConflict):
				conflicted++
			default:
				t.Fatalf("a racing write: %v", err)
			}
		}
		if won != 1 || conflicted != 1 {
			t.Errorf("winners %d, conflicts %d; want one and one", won, conflicted)
		}
	}

	// Two first writes of one setting, both at the initial Version.
	inserts := raceWrites(
		func() error {
			_, err := repo.SaveLibraryOrder(ctx, empty.WithLibraryOrder(settings.NewOrder([]uuid.UUID{firstLibrary})))
			return err
		},
		func() error {
			_, err := repo.SaveLibraryOrder(ctx, empty.WithLibraryOrder(settings.NewOrder([]uuid.UUID{secondLibrary})))
			return err
		},
	)
	checkOneWins(t, inserts)

	stored, ok, err := repo.FindLibraryOrder(ctx, own, org)
	if err != nil || !ok {
		t.Fatalf("Find after the racing inserts: %t, %v", ok, err)
	}
	if stored.Version().Number() != 1 {
		t.Fatalf("the racing inserts stored version %d, want 1", stored.Version().Number())
	}

	// Two updates of the row they both read at that Version.
	updates := raceWrites(
		func() error {
			_, err := repo.SaveLibraryOrder(ctx, stored.WithLibraryOrder(settings.NewOrder([]uuid.UUID{firstLibrary})))
			return err
		},
		func() error {
			_, err := repo.SaveLibraryOrder(ctx, stored.WithLibraryOrder(settings.NewOrder([]uuid.UUID{secondLibrary})))
			return err
		},
	)
	checkOneWins(t, updates)

	stored, ok, err = repo.FindLibraryOrder(ctx, own, org)
	if err != nil || !ok {
		t.Fatalf("Find after the racing updates: %t, %v", ok, err)
	}
	if stored.Version().Number() != 2 {
		t.Errorf("the racing updates stored version %d, want 2", stored.Version().Number())
	}
	if got := stored.LibraryOrder().IDs(); len(got) != 1 {
		t.Errorf("the order after the racing updates: %v", got)
	}
}
