package servertest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// ClientKey is the client key the test helpers act with: the EditLock of a
// Draft holds it while they arrange the Draft through the API.
var ClientKey = uuid.MustParse("c0000000-0000-4000-8000-00000000c11e")

// ClientName is the holder name the test helpers declare.
const ClientName = "servertest"

// EnsureEditLock makes ClientKey the holder of the Project's EditLock in the
// Draft, taking it over from whoever holds it: a test that arranges the Draft
// through the helpers must be able to write it (ADR 0015). A test of the lock
// itself drives the four routes with its own keys instead.
func EnsureEditLock(t *testing.T, srv *httptest.Server, projectID uuid.UUID) editLockState {
	t.Helper()
	path := apiv0.ProjectPath(projectID.String()) + apiv0.EditLockPath
	state := sendJSON[apiv0.EditLockResponse](t, srv, http.MethodGet, path, nil, http.StatusOK)
	if state.Held && state.Holder != nil && state.Holder.Key == ClientKey {
		return editLockState{state}
	}
	if state.Held {
		return editLockState{sendJSON[apiv0.EditLockResponse](t, srv, http.MethodPost, path+"/takeover",
			apiv0.TakeOverEditLockRequest{Name: ClientName, Version: state.Version}, http.StatusOK)}
	}
	return editLockState{sendJSON[apiv0.EditLockResponse](t, srv, http.MethodPost, path,
		apiv0.CheckOutEditLockRequest{Name: ClientName}, http.StatusCreated, http.StatusOK)}
}

// editLockState wraps the lock response of EnsureEditLock so a caller reads
// its fields plainly.
type editLockState struct{ apiv0.EditLockResponse }

// EnsureEditLockWithClient is EnsureEditLock through an API client that acts
// as ClientKey.
func EnsureEditLockWithClient(t *testing.T, client *apiclient.Client, projectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	content := client.Project(projectID)
	state, err := content.StateEditLock(ctx)
	if err != nil {
		t.Fatalf("edit lock of project %s: %v", projectID, err)
	}
	if state.Held && state.Holder != nil && state.Holder.Key == ClientKey {
		return
	}
	if state.Held {
		if _, err := content.TakeOverEditLock(ctx, ClientName, state.Version); err != nil {
			t.Fatalf("take the edit lock of project %s: %v", projectID, err)
		}
		return
	}
	if _, err := content.CheckOutEditLock(ctx, ClientName); err != nil {
		t.Fatalf("check the edit lock of project %s out: %v", projectID, err)
	}
}
