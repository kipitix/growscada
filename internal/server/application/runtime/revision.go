package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// RevisionCause is why a Revision exists: a Deploy made it, a Rollback copied
// an earlier one (ADR 0014), or a project file was deployed as it is
// (ADR 0018).
type RevisionCause string

const (
	// RevisionCauseDeploy is a Revision made from the Draft by a Deploy.
	RevisionCauseDeploy RevisionCause = "deploy"
	// RevisionCauseRollback is a Revision made by copying an earlier one.
	RevisionCauseRollback RevisionCause = "rollback"
	// RevisionCauseFile is a Revision made from a project file, without the
	// Draft (ADR 0018).
	RevisionCauseFile RevisionCause = "file"
)

// ErrRevisionNotFound matches (errors.Is) a read or a Rollback of a Revision
// number the Project does not have.
var ErrRevisionNotFound = errors.New("revision not found")

// ErrNothingToRollback matches (errors.Is) a Rollback refused because the
// Revision it names holds the document the Project runs already (ADR 0014):
// the history would grow with a Revision that changes nothing.
var ErrNothingToRollback = errors.New("nothing to rollback")

// Revision is the Runtime node's view of a Revision: a numbered, immutable
// snapshot of a Project's configuration, created by a Deploy or by a Rollback
// (task 35, ADR 0014). Its Content is the ProjectFile as it was delivered; the
// Runtime node rebuilds its copies of the Tag definitions, the Devices and the
// Environments from it and keeps the Tag states of the Tags that stay
// unchanged. Cause says how it came to be; RolledBackFrom names the Revision a
// Rollback copied, nil for a Deploy.
//
// The Engineering node has its own appdto.Revision for the same document: the
// roles never share their packages (ADR 0011), and the composition of the two
// in one process converts between them.
type Revision struct {
	Number         int
	DeployedAt     time.Time
	Cause          RevisionCause
	RolledBackFrom *int
	Content        projectv0.ProjectFile
}

// RevisionSummary is a Revision without its document: what a list of them
// shows — the number, the time and the cause.
type RevisionSummary struct {
	Number         int
	DeployedAt     time.Time
	Cause          RevisionCause
	RolledBackFrom *int
}

// Summary returns the Revision without its document: what a list of Revisions
// carries.
func (r Revision) Summary() RevisionSummary {
	return RevisionSummary{
		Number:         r.Number,
		DeployedAt:     r.DeployedAt,
		Cause:          r.Cause,
		RolledBackFrom: r.RolledBackFrom,
	}
}

// RevisionRepository stores the Revisions of the Runtime node, the running
// configuration of a Project (task 35). A Revision is append-only: a Project's
// Revisions are numbered 1, 2, 3, … and none is changed or removed while the
// Project lives.
type RevisionRepository interface {
	// CreateRevision numbers and stores the Revision of the Project: one
	// transaction takes the next number, writes the document with cause and
	// brings the Runtime node's copies of the Tag definitions, the Devices,
	// the Environments and their pairs into line with it. The cause is deploy
	// for a Deploy of the Draft and file for a project file (ADR 0018); a
	// Rollback has its own method. The Tag states survive it — a new Tag gets
	// its initial one, a Tag whose TagType changed is reset to it, an
	// unchanged Tag keeps its state — and so do the Device Tokens. Concurrent
	// Deploys of one Project are serialized.
	CreateRevision(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile, cause RevisionCause, now time.Time) (Revision, error)

	// Rollback copies the Revision n of the Project into a new one numbered
	// after the greatest stored: one transaction locks the Project — refusing
	// it while the Project is deleted or being discarded —, compares the
	// document with the running Revision's (equal → ErrNothingToRollback),
	// stores the copy with the cause rollback and the number it copied, and
	// rebuilds the derived copies by the same diff as a Deploy. The document
	// is copied as it is. ErrRevisionNotFound if the Project has no such
	// Revision.
	Rollback(ctx context.Context, projectID uuid.UUID, n int, now time.Time) (Revision, error)

	// FindCurrent returns the latest Revision of the Project, and whether the
	// Project has one at all: a Project is not deployed until it does.
	FindCurrent(ctx context.Context, projectID uuid.UUID) (Revision, bool, error)

	// FindAll returns the summaries of the Project's Revisions, newest first.
	// A Project without a Revision has none; a Project that is gone is
	// project.ErrProjectNotFound.
	FindAll(ctx context.Context, projectID uuid.UUID) ([]RevisionSummary, error)

	// FindByNumber returns the Revision n of the Project with its document.
	// ErrRevisionNotFound if the Project has no such Revision, a Project that
	// is gone is project.ErrProjectNotFound.
	FindByNumber(ctx context.Context, projectID uuid.UUID, n int) (Revision, error)
}
