package runtime

import (
	"context"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// Revision is the Runtime node's view of a Revision: a numbered, immutable
// snapshot of a Project's configuration, created by a Deploy (task 35). Its
// Content is the ProjectFile as it was delivered; the Runtime node rebuilds
// its copies of the Tag definitions, the Devices and the Environments from it
// and keeps the Tag states as they are.
//
// The Engineering node has its own appdto.Revision for the same document: the
// roles never share their packages (ADR 0011), and the composition of the two
// in one process converts between them.
type Revision struct {
	Number     int
	DeployedAt time.Time
	Content    projectv0.ProjectFile
}

// RevisionRepository stores the Revisions of the Runtime node, the running
// configuration of a Project (task 35). A Revision is append-only: a Project's
// Revisions are numbered 1, 2, 3, … and none is changed or removed while the
// Project lives.
type RevisionRepository interface {
	// CreateRevision numbers and stores the Revision of the Project: one
	// transaction takes the next number, writes the document and brings the
	// Runtime node's copies of the Tag definitions, the Devices, the
	// Environments and their pairs into line with it. The Tag states survive
	// it — a new Tag gets its initial one, a Tag whose TagType changed is
	// reset to it, an unchanged Tag keeps its state — and so do the Device
	// Tokens. Concurrent Deploys of one Project are serialized.
	CreateRevision(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile, now time.Time) (Revision, error)

	// FindCurrent returns the latest Revision of the Project, and whether the
	// Project has one at all: a Project is not deployed until it does.
	FindCurrent(ctx context.Context, projectID uuid.UUID) (Revision, bool, error)
}
