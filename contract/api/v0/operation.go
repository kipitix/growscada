package apiv0

// OperationPath is the path, under a Project's path, of its Operation state
// (task 43, ADR 0020): the starting state a Player renders Operation from. It
// is the Runtime node's read model — the Journal position, the running
// Revision and the states of its Tags — read in that order, so a Deploy that
// commits between the reads has a position above the snapshot and reaches the
// live stream.
const OperationPath = "/operation"

// OperationStateResponse is the starting state of Operation: where the
// Project's Journal stands, the Revision it runs and the states of that
// Revision's Tags by Tag ID (ADR 0020).
//
// Position is the Project's current Journal position, read first. Revision is
// the running Revision with its ProjectFile, absent (null) until the Project's
// first Deploy: a live Project answers 200 even then, so Operation can
// subscribe and catch the first Revision. TagStates holds every Tag of the
// Revision, an unwritten one as its TagType's zero value with Quality bad and
// position 0; a Project without a Revision has an empty object.
//
// The Revision's configuration is the ProjectFile as it is: the read model
// builds no render shapes, because the transform matrix and the WidgetType
// content are the Scene's domain on the Engineering node (ADR 0011).
type OperationStateResponse struct {
	Position  int64                       `json:"position"`
	Revision  *RevisionResponse           `json:"revision"`
	TagStates map[string]TagStateResponse `json:"tag_states"`
}
