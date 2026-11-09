package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	recordv0 "github.com/kipitix/growscada/contract/record/v0"
)

const (
	// CheckpointEveryEntries is how far the Journal position of a Project must
	// move past its latest Checkpoint before a periodic snapshot is due. The
	// cost of a replay is the number of entries, not the elapsed time, so the
	// interval is counted in entries (task 44, ADR 0023).
	CheckpointEveryEntries int64 = 10_000
	// CheckpointScanEvery is how often the background service looks for the
	// Projects a snapshot is due for.
	CheckpointScanEvery = time.Minute
)

// CheckpointRepository is the Runtime node's store of Checkpoints (task 44,
// ADR 0023): immutable snapshots of a Project's operational state beside the
// Journal. It is the Runtime node's alone — the Engineering node never reads a
// Checkpoint — so it is not part of its RuntimeNode port.
type CheckpointRepository interface {
	// Nearest returns the latest Checkpoint at or before the position, and
	// whether there is one. A Project that is not live is
	// project.ErrProjectNotFound, not "no checkpoint".
	Nearest(ctx context.Context, projectID uuid.UUID, position int64) (recordv0.Checkpoint, bool, error)
	// DueProjects returns the live Projects whose current Journal position
	// has moved at least everyEntries past their latest Checkpoint.
	DueProjects(ctx context.Context, everyEntries int64) ([]uuid.UUID, error)
	// Snapshot takes one periodic Checkpoint of the Project in a single
	// REPEATABLE READ transaction and reports whether it inserted one: a
	// position that has not moved everyEntries past the latest Checkpoint
	// inserts nothing.
	Snapshot(ctx context.Context, projectID uuid.UUID, everyEntries int64) (bool, error)
}

// CheckpointService is the Runtime node's background writer of periodic
// Checkpoints (task 44, ADR 0023): every CheckpointScanEvery it looks for the
// Projects whose position has moved CheckpointEveryEntries past their latest
// snapshot and takes one snapshot of each. A Deploy, a Rollback and a file
// deployment write their Checkpoint themselves, in their own transaction; this
// service only bounds how far History may have to replay. A failed pass is
// logged, and the next one tries again; no outbox and no policy take part.
type CheckpointService struct {
	checkpoints CheckpointRepository
}

// NewCheckpointService returns the background service over the Checkpoints.
func NewCheckpointService(aCheckpoints CheckpointRepository) *CheckpointService {
	return &CheckpointService{checkpoints: aCheckpoints}
}

// Run scans until ctx is done, once every CheckpointScanEvery.
func (s *CheckpointService) Run(ctx context.Context) {
	ticker := time.NewTicker(CheckpointScanEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
				slog.Error("checkpoints failed to run", "error", err)
			}
		}
	}
}

// RunOnce takes the snapshot of every Project it is due for, once: what a test
// calls instead of starting the ticker, and what Run does on each pass. A
// snapshot that fails is logged and retried on the next pass, so one Project
// never stops the others.
func (s *CheckpointService) RunOnce(ctx context.Context) error {
	projects, err := s.checkpoints.DueProjects(ctx, CheckpointEveryEntries)
	if err != nil {
		return fmt.Errorf("cannot read the projects due for a checkpoint: %w", err)
	}
	for _, projectID := range projects {
		if _, err := s.checkpoints.Snapshot(ctx, projectID, CheckpointEveryEntries); err != nil {
			slog.Error("cannot take a checkpoint", "project", projectID, "error", err)
		}
	}
	return nil
}
