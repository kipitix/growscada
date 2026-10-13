package toast

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestFromError_ByWhereTheCallFailed(t *testing.T) {
	refused := &url.Error{Op: "Get", URL: "http://127.0.0.1:1/api/v0/tags", Err: errors.New("connection refused")}
	badBody := fmt.Errorf("GET /tags: %w: %w", apiclient.ErrBadResponse, errors.New("unexpected EOF"))
	unencodable := errors.New("POST /scenes: cannot encode request: json: unsupported value: NaN")

	for err, title := range map[error]string{
		refused:     "Network Error",
		badBody:     "Bad Response",
		unencodable: "Client Error",
	} {
		want := Problem{Title: title, Detail: err.Error()}
		if got := FromError(err); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	}
}
