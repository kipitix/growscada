package appdto

import (
	"time"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// Revision is the application-layer DTO for a Revision: a numbered,
// immutable snapshot of a Project's configuration, created by a Deploy
// (task 35). Content is the ProjectFile as it was delivered; the Runtime node
// stores the document as it is, and the Engineering node keeps a copy of the
// last one it sent. A Revision is never changed or removed while its Project
// lives.
type Revision struct {
	Number     int
	DeployedAt time.Time
	Content    projectv0.ProjectFile
}
