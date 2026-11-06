package runtime

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	recordv0 "github.com/kipitix/growscada/contract/record/v0"
)

// The size of a Journal page: what a read answers when the caller names no
// limit, and how many entries the live stream catches up by at a time. A page
// never holds more than JournalMaxPageSize entries (task 42).
const (
	JournalPageSize    = 100
	JournalMaxPageSize = 1000
)

// JournalService is the Runtime node's read side of the Journal (task 42,
// ADR 0019): a page of a Project's entries and the Project's current
// position. It is the Runtime node's alone — the Engineering node never reads
// the Journal — so it is not part of the Engineering node's RuntimeNode port.
type JournalService interface {
	// Read returns the Project's entries after the position from, in the
	// order of their positions, at most limit of them. from is exclusive;
	// limit must be between 1 and JournalMaxPageSize and from must not be
	// negative (ErrInvalidInput otherwise). A Project that is not live is
	// project.ErrProjectNotFound, not an empty page.
	Read(ctx context.Context, projectID uuid.UUID, from int64, limit int) ([]recordv0.Entry, error)
	// Position returns the Project's current Journal position — the seq of
	// its latest entry, or 0 for a Project with no entry yet. The live
	// stream starts there when no position is given: only new entries.
	// project.ErrProjectNotFound if the Project is not live.
	Position(ctx context.Context, projectID uuid.UUID) (int64, error)
}

// JournalRepository is the Journal as the Runtime node stores it: rows of
// runtime.journal read in the order of their positions (ADR 0019).
type JournalRepository interface {
	// FindAfter returns the entries of the Project after the position after,
	// in the order of their positions, at most limit of them.
	FindAfter(ctx context.Context, projectID uuid.UUID, after int64, limit int) ([]recordv0.Entry, error)
	// Position returns the seq of the Project's latest entry, 0 if it has
	// none.
	Position(ctx context.Context, projectID uuid.UUID) (int64, error)
}

type journalServiceImpl struct {
	journal JournalRepository
}

var _ JournalService = (*journalServiceImpl)(nil)

// NewJournalService returns the Journal's read side over its repository.
func NewJournalService(journal JournalRepository) JournalService {
	return &journalServiceImpl{journal: journal}
}

func (s journalServiceImpl) Read(ctx context.Context, projectID uuid.UUID, from int64, limit int) ([]recordv0.Entry, error) {
	if from < 0 {
		return nil, invalidInput(fmt.Errorf("journal position %d is negative", from))
	}
	if limit <= 0 || limit > JournalMaxPageSize {
		return nil, invalidInput(fmt.Errorf("journal page size %d is not between 1 and %d", limit, JournalMaxPageSize))
	}
	entries, err := s.journal.FindAfter(ctx, projectID, from, limit)
	if err != nil {
		return nil, fmt.Errorf("cannot read the journal of project %s after %d: %w", projectID, from, err)
	}
	return entries, nil
}

func (s journalServiceImpl) Position(ctx context.Context, projectID uuid.UUID) (int64, error) {
	position, err := s.journal.Position(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("cannot read the journal position of project %s: %w", projectID, err)
	}
	return position, nil
}
