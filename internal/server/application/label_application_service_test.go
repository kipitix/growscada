package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/label"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func newLabelService(t *testing.T) application.LabelService {
	t.Helper()
	org, err := repositories.FindSoleOrganization(context.Background(), testDB)
	if err != nil {
		t.Fatalf("organization: %v", err)
	}
	return application.NewLabelService(repositories.NewLabelRepositoryPostgres(testDB),
		repositories.NewDraftRepositoryPostgres(testDB, repositories.NewLibraryReleaseRepositoryPostgres(testDB)), org)
}

// TestLabelService_CreateUpdateDelete covers the service of the labels: the
// name's uniqueness ignoring case, the optional color, an update that keeps
// what it does not state, and the delete (task 85_2; ADR 0025).
func TestLabelService_CreateUpdateDelete(t *testing.T) {
	ctx := context.Background()
	svc := newLabelService(t)
	name := "test-" + uuid.NewString()[:8]

	created, err := svc.Create(ctx, "  "+name+"  ", "#A1B2C3")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Name != name || created.Color != "#a1b2c3" || created.AssignedCount != 0 || created.ID == uuid.Nil || created.CreatedAt.IsZero() {
		t.Errorf("the created label: %+v", created)
	}

	if _, err := svc.Create(ctx, strings.ToUpper(name), ""); !errors.Is(err, label.ErrLabelNameTaken) {
		t.Errorf("a taken name: %v, want ErrLabelNameTaken", err)
	}
	if _, err := svc.Create(ctx, " ", ""); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("an empty name: %v, want ErrInvalidInput", err)
	}
	if _, err := svc.Create(ctx, name+"-bad", "#12345"); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("a bad color: %v, want ErrInvalidInput", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, l := range list {
		if l.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("List misses the created label: %+v", list)
	}

	// A name alone keeps the color; a nil field keeps its own.
	renamed, err := svc.Update(ctx, created.ID, ptr(name+"-renamed"), nil)
	if err != nil {
		t.Fatalf("Update the name: %v", err)
	}
	if renamed.Name != name+"-renamed" || renamed.Color != "#a1b2c3" {
		t.Errorf("the renamed label: %+v", renamed)
	}
	// A color alone keeps the name; an empty one clears it.
	recolored, err := svc.Update(ctx, created.ID, nil, ptr(""))
	if err != nil {
		t.Fatalf("Update the color: %v", err)
	}
	if recolored.Name != name+"-renamed" || recolored.Color != "" {
		t.Errorf("the recolored label: %+v", recolored)
	}
	// Nothing stated is nothing written: the label comes back as it is.
	same, err := svc.Update(ctx, created.ID, nil, nil)
	if err != nil || same.Name != recolored.Name || same.Color != recolored.Color {
		t.Errorf("an update with nothing stated: %+v, %v", same, err)
	}
	if _, err := svc.Update(ctx, created.ID, ptr(" "), nil); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("an empty rename: %v, want ErrInvalidInput", err)
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, label.ErrLabelNotFound) {
		t.Errorf("a second delete: %v, want ErrLabelNotFound", err)
	}
	if _, err := svc.Update(ctx, uuid.New(), ptr("elsewhere"), nil); !errors.Is(err, label.ErrLabelNotFound) {
		t.Errorf("an update of a missing label: %v, want ErrLabelNotFound", err)
	}
}

func ptr(s string) *string { return &s }
