package restapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// orderPath is the route of the per-user library order (task 81).
var orderPath = apiv0.PathPrefix + apiv0.LibraryOrderPath

// TestSettings_LibraryOrder covers the per-user library order (task 81,
// ADR 0024): a person who never reordered, their first write, the Version it
// raises, the refusal of a stale one, and the tolerance of a library that does
// not exist.
func TestSettings_LibraryOrder(t *testing.T) {
	if _, err := testDB().ExecContext(context.Background(), "DELETE FROM user_settings"); err != nil {
		t.Fatalf("clean the settings: %v", err)
	}
	services := newServices()
	router := services.router()

	rec := serve(router, http.MethodGet, orderPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET library order: %d %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"library_ids":[]`) || !strings.Contains(body, `"version":0`) {
		t.Errorf("an order nobody wrote: %s", body)
	}

	first := uuid.MustParse("d0000000-0000-4000-8000-000000000001")
	second := uuid.MustParse("d0000000-0000-4000-8000-000000000002")
	rec = serve(router, http.MethodPut, orderPath, apiv0.SetLibraryOrderRequest{
		Version: 0,
		// A repetition and the zero ID are dropped, not refused (ADR 0024).
		LibraryIDs: []uuid.UUID{second, second, uuid.Nil, first},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT the first order: %d %s", rec.Code, rec.Body)
	}
	written := decodeBody[apiv0.LibraryOrderResponse](t, rec)
	if written.Version != 1 {
		t.Errorf("the first write answered version %d, want 1", written.Version)
	}
	if len(written.LibraryIDs) != 2 || written.LibraryIDs[0] != second || written.LibraryIDs[1] != first {
		t.Errorf("the normalized order: %v", written.LibraryIDs)
	}

	read := decodeBody[apiv0.LibraryOrderResponse](t, serve(router, http.MethodGet, orderPath, nil))
	if read.Version != 1 || len(read.LibraryIDs) != 2 || read.LibraryIDs[1] != first {
		t.Errorf("the stored order: %+v", read)
	}

	// library_ids is required, as the schema states: a body without it is a
	// client error, not an empty order, and the stored arrangement stays.
	rec = serve(router, http.MethodPut, orderPath, map[string]any{"version": 1})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT without library_ids: %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("a refusal's content type: %q", got)
	}
	read = decodeBody[apiv0.LibraryOrderResponse](t, serve(router, http.MethodGet, orderPath, nil))
	if read.Version != 1 || len(read.LibraryIDs) != 2 {
		t.Errorf("a refused body changed the order: %+v", read)
	}

	// A write at a stale Version is 409 with a Problem, and changes nothing.
	rec = serve(router, http.MethodPut, orderPath, apiv0.SetLibraryOrderRequest{
		Version: 0, LibraryIDs: []uuid.UUID{first},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("PUT at a stale version: %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("a refusal's content type: %q", got)
	}
	read = decodeBody[apiv0.LibraryOrderResponse](t, serve(router, http.MethodGet, orderPath, nil))
	if read.Version != 1 || len(read.LibraryIDs) != 2 {
		t.Errorf("a refused write changed the order: %+v", read)
	}

	// The ID of a library that was really deleted is tolerated: the order is a
	// view of the libraries, never an invariant about them (ADR 0024).
	gone, err := services.libraries.CreateLibrary(context.Background(), appdto.LibraryInput{Name: "Gone"})
	if err != nil {
		t.Fatalf("create a library to delete: %v", err)
	}
	if _, err := services.libraries.DeleteLibraryByID(context.Background(), gone.ID); err != nil {
		t.Fatalf("delete the library: %v", err)
	}
	rec = serve(router, http.MethodPut, orderPath, apiv0.SetLibraryOrderRequest{
		Version: 1, LibraryIDs: []uuid.UUID{gone.ID, first},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT with a deleted library: %d %s", rec.Code, rec.Body)
	}
	read = decodeBody[apiv0.LibraryOrderResponse](t, serve(router, http.MethodGet, orderPath, nil))
	if read.Version != 2 || len(read.LibraryIDs) != 2 || read.LibraryIDs[0] != gone.ID {
		t.Errorf("the order with a deleted library: %+v", read)
	}
}
