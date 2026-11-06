package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// fakeJournal is a JournalRepository handing back what a test gave it and
// remembering the range it was asked for.
type fakeJournal struct {
	entries     []recordv0.Entry
	findErr     error
	position    int64
	positionErr error
	projectID   uuid.UUID
	after       int64
	limit       int
}

func (j *fakeJournal) FindAfter(_ context.Context, projectID uuid.UUID, after int64, limit int) ([]recordv0.Entry, error) {
	j.projectID, j.after, j.limit = projectID, after, limit
	return j.entries, j.findErr
}

func (j *fakeJournal) Position(_ context.Context, projectID uuid.UUID) (int64, error) {
	j.projectID = projectID
	return j.position, j.positionErr
}

func TestJournalService_Read_AsksForTheRangeAndReturnsItsEntries(t *testing.T) {
	projectID := uuid.New()
	entries := []recordv0.Entry{{Seq: 8, ProjectID: projectID}, {Seq: 9, ProjectID: projectID}}
	journal := &fakeJournal{entries: entries}
	service := runtimeapp.NewJournalService(journal)

	found, err := service.Read(context.Background(), projectID, 7, 2)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(found) != 2 || found[0].Seq != 8 || found[1].Seq != 9 {
		t.Errorf("Read returned %+v", found)
	}
	if journal.projectID != projectID || journal.after != 7 || journal.limit != 2 {
		t.Errorf("asked for %s after %d limit %d", journal.projectID, journal.after, journal.limit)
	}
}

func TestJournalService_Read_RefusesAPositionAndAPageItCannotServe(t *testing.T) {
	service := runtimeapp.NewJournalService(&fakeJournal{})
	for _, tc := range []struct {
		name  string
		from  int64
		limit int
	}{
		{"negative position", -1, runtimeapp.JournalPageSize},
		{"zero limit", 0, 0},
		{"negative limit", 0, -1},
		{"limit above the maximum", 0, runtimeapp.JournalMaxPageSize + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Read(context.Background(), uuid.New(), tc.from, tc.limit); !errors.Is(err, runtimeapp.ErrInvalidInput) {
				t.Errorf("%v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestJournalService_Read_PassesTheProjectsFateThrough(t *testing.T) {
	journal := &fakeJournal{findErr: project.ErrProjectNotFound}
	service := runtimeapp.NewJournalService(journal)
	if _, err := service.Read(context.Background(), uuid.New(), 0, runtimeapp.JournalPageSize); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("%v, want project.ErrProjectNotFound", err)
	}
}

func TestJournalService_Position_ReturnsTheProjectsPosition(t *testing.T) {
	projectID := uuid.New()
	service := runtimeapp.NewJournalService(&fakeJournal{position: 42})
	position, err := service.Position(context.Background(), projectID)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}
	if position != 42 {
		t.Errorf("position %d, want 42", position)
	}
}

func TestJournalService_Position_PassesTheProjectsFateThrough(t *testing.T) {
	journal := &fakeJournal{positionErr: project.ErrProjectNotFound}
	service := runtimeapp.NewJournalService(journal)
	if _, err := service.Position(context.Background(), uuid.New()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("%v, want project.ErrProjectNotFound", err)
	}
}
