package restapi

import (
	"fmt"
	"net/http"
	"strconv"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// OperationHandlers serves the starting state of Operation (task 43,
// ADR 0020): the Project's Journal position, the Deployment it runs and the
// states of that Deployment's Tags. It is the Runtime node's model; the handler
// touches no Draft and holds no lock.
type OperationHandlers struct {
	operation runtimeapp.OperationStateService
}

// NewOperationHandler creates the handler over the Runtime node's Operation
// read model.
func NewOperationHandler(operation runtimeapp.OperationStateService) *OperationHandlers {
	return &OperationHandlers{operation: operation}
}

// GetOperation handles GET /projects/{project_id}/operation: 200 with the
// Operation state. A live Project answers 200 even before its first Deploy —
// deployment null, tag_states {} — so Operation can subscribe and catch the
// first Deployment; a Project that is not live is 404. The optional at names a
// past Journal position (task 44, ADR 0023): the answer is then the nearest
// Checkpoint at or before it, and position is that Checkpoint's. A position
// that is not a number is 400, and so is a negative one.
func (h OperationHandlers) GetOperation(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	var (
		state runtimeapp.OperationState
		err   error
	)
	if raw := r.URL.Query().Get(apiv0.OperationQueryAt); raw == "" {
		state, err = h.operation.Get(r.Context(), projectID)
	} else {
		at, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' is not a number", apiv0.OperationQueryAt, raw), r.URL.Path))
			return
		}
		state, err = h.operation.GetAt(r.Context(), projectID, at)
	}
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newOperationStateResponse(state))
}
