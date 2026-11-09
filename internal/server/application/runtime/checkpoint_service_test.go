package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// fakeCheckpoints is a CheckpointRepository over the Projects a test makes due,
// remembering the snapshots it was asked to take.
type fakeCheckpoints struct {
	due         []uuid.UUID
	dueErr      error
	snapshotErr map[uuid.UUID]error
	snapshots   []uuid.UUID
	every       []int64

	// current models the store: a Project leaves due once its snapshot is
	// taken, so a second pass finds nothing to do.
}

func (c *fakeCheckpoints) Nearest(_ context.Context, projectID uuid.UUID, position int64) (recordv0.Checkpoint, bool, error) {
	return recordv0.Checkpoint{}, false, nil
}

func (c *fakeCheckpoints) DueProjects(_ context.Context, everyEntries int64) ([]uuid.UUID, error) {
	c.every = append(c.every, everyEntries)
	return c.due, c.dueErr
}

func (c *fakeCheckpoints) Snapshot(_ context.Context, projectID uuid.UUID, everyEntries int64) (bool, error) {
	c.every = append(c.every, everyEntries)
	if err, ok := c.snapshotErr[projectID]; ok {
		return false, err
	}
	c.snapshots = append(c.snapshots, projectID)
	c.remove(projectID)
	return true, nil
}

func (c *fakeCheckpoints) remove(projectID uuid.UUID) {
	remaining := c.due[:0]
	for _, candidate := range c.due {
		if candidate != projectID {
			remaining = append(remaining, candidate)
		}
	}
	c.due = remaining
}

// TestCheckpointService_RunOnceTakesOneSnapshotPerDueProject: the service takes
// the snapshot of every Project the repository names due, with the entry
// threshold, and a second pass adds nothing (task 44).
func TestCheckpointService_RunOnceTakesOneSnapshotPerDueProject(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	checkpoints := &fakeCheckpoints{due: []uuid.UUID{first, second}}
	service := runtimeapp.NewCheckpointService(checkpoints)

	if err := service.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(checkpoints.snapshots) != 2 || checkpoints.snapshots[0] != first || checkpoints.snapshots[1] != second {
		t.Errorf("snapshots %v, want %s and %s", checkpoints.snapshots, first, second)
	}
	for _, every := range checkpoints.every {
		if every != runtimeapp.CheckpointEveryEntries {
			t.Errorf("threshold %d, want %d", every, runtimeapp.CheckpointEveryEntries)
		}
	}

	if err := service.RunOnce(context.Background()); err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}
	if len(checkpoints.snapshots) != 2 {
		t.Errorf("a second pass took %v, want nothing new", checkpoints.snapshots)
	}
}

// TestCheckpointService_RunOnceKeepsGoingAfterAFailedSnapshot: one Project's
// failure is retried on the next pass and never stops the others (ADR 0023).
func TestCheckpointService_RunOnceKeepsGoingAfterAFailedSnapshot(t *testing.T) {
	failing, healthy := uuid.New(), uuid.New()
	checkpoints := &fakeCheckpoints{
		due:         []uuid.UUID{failing, healthy},
		snapshotErr: map[uuid.UUID]error{failing: errors.New("snapshot failed")},
	}
	service := runtimeapp.NewCheckpointService(checkpoints)
	if err := service.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(checkpoints.snapshots) != 1 || checkpoints.snapshots[0] != healthy {
		t.Errorf("snapshots %v, want %s alone", checkpoints.snapshots, healthy)
	}
}

// TestCheckpointService_RunOncePassesTheLookupFailureThrough: a repository that
// cannot name the due Projects is an error, logged by Run.
func TestCheckpointService_RunOncePassesTheLookupFailureThrough(t *testing.T) {
	checkpoints := &fakeCheckpoints{dueErr: errors.New("read failed")}
	service := runtimeapp.NewCheckpointService(checkpoints)
	if err := service.RunOnce(context.Background()); err == nil {
		t.Error("RunOnce hid a failed lookup")
	}
}
