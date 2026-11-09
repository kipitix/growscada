package apiv0

// OperationPath is the path, under a Project's path, of its Operation state
// (task 43, ADR 0020): the starting state a Player renders Operation from. It
// is the Runtime node's read model — the Journal position, the running
// Deployment and the states of its Tags — read in that order, so a Deploy that
// commits between the reads has a position above the snapshot and reaches the
// live stream.
const OperationPath = "/operation"

// OperationQueryAt is the optional parameter of GET …/operation (task 44,
// ADR 0023): the Journal position to answer the starting state at, for History.
// The answer is the nearest Checkpoint at or before it, read like a deployment
// document, with the Checkpoint's position in the answer; a position beyond the
// Project's current one is clamped to it, and a negative or non-numeric one is
// 400. Without it the route reads the present state, as before.
const OperationQueryAt = "at"

// OperationStateResponse is the starting state of Operation: where the
// Project's Journal stands, the Deployment it runs and the states of that
// Deployment's Tags by Tag ID (ADR 0020).
//
// Position is the Project's current Journal position, read first. Deployment is
// the running Deployment with its ProjectFile, absent (null) until the Project's
// first Deploy: a live Project answers 200 even then, so Operation can
// subscribe and catch the first Deployment. TagStates holds every Tag of the
// Deployment, an unwritten one as its TagType's zero value with Quality bad and
// position 0; a Project without a Deployment has an empty object.
//
// The Deployment's configuration is the ProjectFile as it is: the read model
// builds no render shapes, because the transform matrix and the WidgetType
// content are the Scene's domain on the Engineering node (ADR 0011).
type OperationStateResponse struct {
	Position   int64                       `json:"position"`
	Deployment *DeploymentResponse         `json:"deployment"`
	TagStates  map[string]TagStateResponse `json:"tag_states"`
}
