package toast

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/apiclient"
)

func TestFromError_ServersProblem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"type":"t","title":"Resource Not Found","status":404,
			"detail":"Tag with ID 'x' not found","instance":"/api/v0/tags/x"}`))
	}))
	defer srv.Close()
	_, err := apiclient.New(srv.URL, srv.Client()).DeleteTag(context.Background(), uuid.New())

	// Wrapped, as a caller may do.
	got := FromError(fmt.Errorf("delete: %w", err))

	want := Problem{Title: "Resource Not Found", Status: 404, Detail: "Tag with ID 'x' not found",
		Instance: "/api/v0/tags/x", Method: http.MethodDelete}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFromError_AnythingElseIsANetworkError(t *testing.T) {
	got := FromError(errors.New("dial tcp 127.0.0.1:1: connection refused"))

	want := Problem{Title: "Network Error", Detail: "dial tcp 127.0.0.1:1: connection refused"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
