package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// DeploymentCause is why a Deployment exists: a Deploy made it from a Release
// (task 72_4), a Rollback copied an earlier one (ADR 0014), or a project file
// was deployed as it is (ADR 0018).
type DeploymentCause string

const (
	// DeploymentCauseDeploy is a Deployment made from a Project Release by a
	// Deploy.
	DeploymentCauseDeploy DeploymentCause = "deploy"
	// DeploymentCauseRollback is a Deployment made by copying an earlier one.
	DeploymentCauseRollback DeploymentCause = "rollback"
	// DeploymentCauseFile is a Deployment made from a project file, without the
	// Draft (ADR 0018).
	DeploymentCauseFile DeploymentCause = "file"
)

// ErrDeploymentNotFound matches (errors.Is) a read or a Rollback of a Deployment
// number the Project does not have.
var ErrDeploymentNotFound = errors.New("deployment not found")

// ErrNothingToRollback matches (errors.Is) a Rollback refused because the
// Deployment it names holds the document the Project runs already (ADR 0014):
// the history would grow with a Deployment that changes nothing.
var ErrNothingToRollback = errors.New("nothing to rollback")

// ErrNothingToDeploy matches (errors.Is) a Deploy Release refused because the
// Project already runs that Release (task 72_4): the Release is immutable, so
// deploying it again would add a Deployment that changes nothing.
var ErrNothingToDeploy = errors.New("nothing to deploy")

// Deployment is the Runtime node's view of a Deployment: a numbered, immutable
// snapshot of a Project's configuration, created by a Deploy or by a Rollback
// (task 35, ADR 0014). Its Content is the ProjectFile as it was delivered; the
// Runtime node rebuilds its copies of the Tag definitions, the Devices and the
// Environments from it and keeps the Tag states of the Tags that stay
// unchanged. Cause says how it came to be; RolledBackFrom names the Deployment a
// Rollback copied, nil for a Deploy.
//
// The Engineering node has its own appdto.Deployment for the same document: the
// roles never share their packages (ADR 0011), and the composition of the two
// in one process converts between them.
type Deployment struct {
	Number         int
	DeployedAt     time.Time
	Cause          DeploymentCause
	RolledBackFrom *int
	// ReleaseNumber is the Project Release the Deployment made (task 72_4):
	// nil for a Deployment from a file, or one made before Releases existed.
	ReleaseNumber *int
	Content       projectv0.ProjectFile
}

// DeploymentSummary is a Deployment without its document: what a list of them
// shows — the number, the time, the cause and the Release.
type DeploymentSummary struct {
	Number         int
	DeployedAt     time.Time
	Cause          DeploymentCause
	RolledBackFrom *int
	ReleaseNumber  *int
}

// Summary returns the Deployment without its document: what a list of Deployments
// carries.
func (r Deployment) Summary() DeploymentSummary {
	return DeploymentSummary{
		Number:         r.Number,
		DeployedAt:     r.DeployedAt,
		Cause:          r.Cause,
		RolledBackFrom: r.RolledBackFrom,
		ReleaseNumber:  r.ReleaseNumber,
	}
}

// DeploymentRepository stores the Deployments of the Runtime node, the running
// configuration of a Project (task 35). A Deployment is append-only: a Project's
// Deployments are numbered 1, 2, 3, … and none is changed or removed while the
// Project lives.
type DeploymentRepository interface {
	// CreateDeployment numbers and stores the Deployment of the Project: one
	// transaction takes the next number, writes the document with cause and
	// releaseNumber and brings the Runtime node's copies of the Tag
	// definitions, the Devices, the Environments and their pairs into line
	// with it. The cause is deploy for a Deploy of a Release and file for a
	// project file (ADR 0018); a Rollback has its own method. releaseNumber is
	// the Project Release the document came from, nil for a file deployment.
	// A Deploy of the Release the Project already runs is ErrNothingToDeploy,
	// checked under the same lock that numbers the Deployment. The Tag states
	// survive it — a new Tag gets its initial one, a Tag whose TagType changed
	// is reset to it, an unchanged Tag keeps its state — and so do the Device
	// Tokens. Concurrent Deploys of one Project are serialized.
	CreateDeployment(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile, cause DeploymentCause, releaseNumber *int, now time.Time) (Deployment, error)

	// Rollback copies the Deployment n of the Project into a new one numbered
	// after the greatest stored: one transaction locks the Project — refusing
	// it while the Project is deleted or being discarded —, compares the
	// document with the running Deployment's (equal → ErrNothingToRollback),
	// stores the copy with the cause rollback, the number it copied and its
	// Release, and rebuilds the derived copies by the same diff as a Deploy.
	// The document is copied as it is. ErrDeploymentNotFound if the Project
	// has no such Deployment.
	Rollback(ctx context.Context, projectID uuid.UUID, n int, now time.Time) (Deployment, error)

	// FindCurrent returns the latest Deployment of the Project, and whether the
	// Project has one at all: a Project is not deployed until it does.
	FindCurrent(ctx context.Context, projectID uuid.UUID) (Deployment, bool, error)

	// FindAll returns the summaries of the Project's Deployments, newest first.
	// A Project without a Deployment has none; a Project that is gone is
	// project.ErrProjectNotFound.
	FindAll(ctx context.Context, projectID uuid.UUID) ([]DeploymentSummary, error)

	// FindByNumber returns the Deployment n of the Project with its document.
	// ErrDeploymentNotFound if the Project has no such Deployment, a Project that
	// is gone is project.ErrProjectNotFound.
	FindByNumber(ctx context.Context, projectID uuid.UUID, n int) (Deployment, error)

	// DeploymentNamesLibrary reports whether the running Deployment of any live
	// Project names a Release of this Library in its ProjectFile (task 71): the
	// Engineering node asks before a Library is removed, because a Discard of
	// such a Deployment could not restore the pin. A Project that runs no
	// Deployment names nothing.
	DeploymentNamesLibrary(ctx context.Context, libraryID uuid.UUID) (bool, error)
}
