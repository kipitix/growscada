package library

import (
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/servertest"
)

// testAPI is a second client of the test server, which arranges and checks
// its state as another user would: the server's own resources, and the
// content of the project Main the tests work in.
type testAPI struct {
	*apiclient.Client
	*apiclient.ProjectClient
}

func newTestAPI(client *apiclient.Client) *testAPI {
	return &testAPI{Client: client, ProjectClient: client.Project(servertest.MainProject)}
}
