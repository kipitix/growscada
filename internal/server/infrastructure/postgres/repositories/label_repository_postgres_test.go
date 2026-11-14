package repositories_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/label"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// cleanTestLabels removes the labels the tests create, leaving the migrations'
// seed labels alone.
func cleanTestLabels(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`DELETE FROM engineering.labels WHERE name LIKE 'test-%'`); err != nil {
		t.Fatalf("cleanTestLabels: %v", err)
	}
}

func mustLabelName(t *testing.T, raw string) label.Name {
	t.Helper()
	name, err := label.NewName(raw)
	if err != nil {
		t.Fatalf("NewName(%q): %v", raw, err)
	}
	return name
}

func mustLabelColor(t *testing.T, raw string) label.Color {
	t.Helper()
	color, err := label.NewColor(raw)
	if err != nil {
		t.Fatalf("NewColor(%q): %v", raw, err)
	}
	return color
}

// TestLabels_CRUDAndCaseInsensitiveUniqueness covers the life of a label and
// the Organization-wide uniqueness of its name (task 85_2; ADR 0025).
func TestLabels_CRUDAndCaseInsensitiveUniqueness(t *testing.T) {
	cleanTestLabels(t)
	ctx := context.Background()
	repo := repositories.NewLabelRepositoryPostgres(testDB)
	org := soleOrganization(t)
	name := "test-" + uuid.NewString()[:8]
	created := label.CreateLabel(repo.NextID(), org, mustLabelName(t, name), mustLabelColor(t, "#A1B2C3"), time.Now())
	saved, err := repo.Create(ctx, created)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.Name().String() != name || saved.Color().String() != "#a1b2c3" {
		t.Errorf("the stored label: %s", saved)
	}

	// The name is one in the Organization, whatever its case.
	duplicate := label.CreateLabel(repo.NextID(), org, mustLabelName(t, strings.ToUpper(name)), label.Color{}, time.Now())
	if _, err := repo.Create(ctx, duplicate); !errors.Is(err, label.ErrLabelNameTaken) {
		t.Errorf("a taken name: %v, want ErrLabelNameTaken", err)
	}

	found, err := repo.FindByID(ctx, org, saved.ID())
	if err != nil || found.ID() != saved.ID() {
		t.Fatalf("FindByID: %s, %v", found, err)
	}
	all, err := repo.FindAll(ctx, org)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if !containsLabel(all, saved.ID()) {
		t.Errorf("FindAll misses the created label: %v", all)
	}

	// A rename that only changes the case does not conflict with itself.
	renamed, err := found.Rename(mustLabelName(t, strings.ToUpper(name)))
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	updated, err := repo.Update(ctx, renamed)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name().String() != strings.ToUpper(name) {
		t.Errorf("the renamed label: %s", updated)
	}

	// Another label's name is taken, whatever its case.
	other := label.CreateLabel(repo.NextID(), org, mustLabelName(t, name+"-other"), label.Color{}, time.Now())
	if _, err := repo.Create(ctx, other); err != nil {
		t.Fatalf("Create another: %v", err)
	}
	clash, err := repo.FindByID(ctx, org, other.ID())
	if err != nil {
		t.Fatalf("FindByID another: %v", err)
	}
	clash, err = clash.Rename(mustLabelName(t, strings.ToUpper(name)))
	if err != nil {
		t.Fatalf("Rename another: %v", err)
	}
	if _, err := repo.Update(ctx, clash); !errors.Is(err, label.ErrLabelNameTaken) {
		t.Errorf("a rename to a taken name: %v, want ErrLabelNameTaken", err)
	}

	// The recolor is stored, an empty color as NULL.
	recolored, err := updated.Recolor(mustLabelColor(t, ""))
	if err != nil {
		t.Fatalf("Recolor: %v", err)
	}
	stored, err := repo.Update(ctx, recolored)
	if err != nil {
		t.Fatalf("Update the cleared color: %v", err)
	}
	if !stored.Color().IsEmpty() {
		t.Errorf("the cleared color: %q, want none", stored.Color())
	}

	// The outbox carries the events of the changes.
	var events int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM outbox WHERE type LIKE 'label_%' AND payload->>'id' = $1`,
		saved.ID().UUID().String(),
	).Scan(&events); err != nil {
		t.Fatalf("read the outbox: %v", err)
	}
	if events != 3 {
		t.Errorf("the label's outbox rows: %d, want label_created, label_updated and label_updated", events)
	}

	if err := repo.Delete(ctx, stored); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, org, saved.ID()); !errors.Is(err, label.ErrLabelNotFound) {
		t.Errorf("a read of the deleted label: %v, want ErrLabelNotFound", err)
	}
	if err := repo.Delete(ctx, stored); !errors.Is(err, label.ErrLabelNotFound) {
		t.Errorf("a second delete: %v, want ErrLabelNotFound", err)
	}
}

func containsLabel(list []label.Label, labelID id.ID[label.Label]) bool {
	for _, l := range list {
		if l.ID() == labelID {
			return true
		}
	}
	return false
}

// TestLabels_OrganizationCascade: labels belong to their Organization, so
// removing it removes them with it (task 85_2; ADR 0025).
func TestLabels_OrganizationCascade(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewLabelRepositoryPostgres(testDB)
	orgUUID := uuid.New()
	if _, err := testDB.ExecContext(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`,
		orgUUID, "test-org-"+orgUUID.String()[:8]); err != nil {
		t.Fatalf("insert an organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgUUID)
	})
	org := id.NewID(id.IDWithUUID[organization.Organization](orgUUID))
	created, err := repo.Create(ctx, label.CreateLabel(repo.NextID(), org,
		mustLabelName(t, "test-cascade"), label.Color{}, time.Now()))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, `DELETE FROM organizations WHERE id = $1`, orgUUID); err != nil {
		t.Fatalf("delete the organization: %v", err)
	}
	if _, err := repo.FindByID(ctx, org, created.ID()); !errors.Is(err, label.ErrLabelNotFound) {
		t.Errorf("the label survived its Organization: %v, want ErrLabelNotFound", err)
	}
}
