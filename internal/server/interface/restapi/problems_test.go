package restapi_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// The body on the wire is what the hand-written marshaller of the former
// restapi.ProblemDetails produced: the RFC 9457 members, empty optional ones
// left out.
func TestProblem_MarshalJSON_KeepsTheWireShape(t *testing.T) {
	data, err := json.Marshal(restapi.NewBadRequest("something went wrong", "/api/v0/tags"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]any{
		"type":     restapi.TypeBadRequest,
		"title":    "Bad Request",
		"status":   float64(400),
		"detail":   "something went wrong",
		"instance": "/api/v0/tags",
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("got %v, want %v", result, want)
	}
}

func TestProblem_MarshalJSON_OmitsEmptyDetailAndInstance(t *testing.T) {
	data, err := json.Marshal(restapi.NewBadRequest("", ""))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"detail", "instance"} {
		if _, ok := result[key]; ok {
			t.Errorf("%s should be omitted when empty", key)
		}
	}
}

func TestNewBadRequest_HasCorrectFields(t *testing.T) {
	p := restapi.NewBadRequest("bad input", "/api/v0/tags")

	if p.Status != http.StatusBadRequest {
		t.Errorf("Status: expected 400, got %d", p.Status)
	}
	if p.Type != restapi.TypeBadRequest {
		t.Errorf("Type: expected %q, got %q", restapi.TypeBadRequest, p.Type)
	}
	if p.Detail != "bad input" {
		t.Errorf("Detail: expected %q, got %q", "bad input", p.Detail)
	}
	if p.Instance != "/api/v0/tags" {
		t.Errorf("Instance: expected %q, got %q", "/api/v0/tags", p.Instance)
	}
}

func TestNewNotFound_HasCorrectStatusAndType(t *testing.T) {
	p := restapi.NewNotFound("tag", "abc-123", "/api/v0/tags/abc-123")

	if p.Status != http.StatusNotFound {
		t.Errorf("Status: expected 404, got %d", p.Status)
	}
	if p.Type != restapi.TypeNotFound {
		t.Errorf("Type: expected %q, got %q", restapi.TypeNotFound, p.Type)
	}
	if p.Detail == "" {
		t.Error("Detail should not be empty")
	}
}

func TestNewConflict_HasCorrectStatusAndType(t *testing.T) {
	p := restapi.NewConflict("tag", "already exists", "/api/v0/tags")

	if p.Status != http.StatusConflict {
		t.Errorf("Status: expected 409, got %d", p.Status)
	}
	if p.Type != restapi.TypeConflict {
		t.Errorf("Type: expected %q, got %q", restapi.TypeConflict, p.Type)
	}
}

func TestNewInternalError_HasCorrectFields(t *testing.T) {
	p := restapi.NewInternalError("database error", "/api/v0/tags")

	if p.Status != http.StatusInternalServerError {
		t.Errorf("Status: expected 500, got %d", p.Status)
	}
	if p.Type != restapi.TypeInternalError {
		t.Errorf("Type: expected %q, got %q", restapi.TypeInternalError, p.Type)
	}
	if p.Detail != "database error" {
		t.Errorf("Detail: expected %q, got %q", "database error", p.Detail)
	}
}
