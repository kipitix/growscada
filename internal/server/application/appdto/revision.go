package appdto

import (
	"time"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// Revision is the application-layer DTO for a Revision: a numbered,
// immutable snapshot of a Project's configuration, created by a Deploy or by
// a Rollback (task 35, ADR 0014). Content is the ProjectFile as it was
// delivered; the Runtime node stores the document as it is. Cause is "deploy"
// or "rollback"; RolledBackFrom names the Revision a Rollback copied, nil for
// a Deploy. A Revision is never changed or removed while its Project lives.
type Revision struct {
	Number         int
	DeployedAt     time.Time
	Cause          string
	RolledBackFrom *int
	Content        projectv0.ProjectFile
}
