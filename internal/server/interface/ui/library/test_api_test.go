package library

import (
	"context"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// testAPI is a second client of the test server, which arranges and checks
// its state as another user would: the server's own resources, and the
// library "Widgets" the tests work in, the server's only one.
type testAPI struct {
	*apiclient.Client
	*apiclient.LibraryClient
}

func newTestAPI(t *testing.T, client *apiclient.Client) *testAPI {
	t.Helper()
	lib, err := client.CreateLibrary(context.Background(), apiv0.CreateLibraryRequest{Name: "Widgets"})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	return &testAPI{Client: client, LibraryClient: client.Library(lib.ID)}
}
