package restapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kipitix/growscada/internal/interface/restapi"
)

func TestProblemDetails_MarshalJSON_ContainsStandardFields(t *testing.T) {
	p := restapi.ProblemDetails{
		Type:   "https://example.com/error",
		Title:  "Test Error",
		Status: 400,
		Detail: "something went wrong",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result["type"] != "https://example.com/error" {
		t.Errorf("type: expected %q, got %v", "https://example.com/error", result["type"])
	}
	if result["title"] != "Test Error" {
		t.Errorf("title: expected %q, got %v", "Test Error", result["title"])
	}
	if int(result["status"].(float64)) != 400 {
		t.Errorf("status: expected 400, got %v", result["status"])
	}
	if result["detail"] != "something went wrong" {
		t.Errorf("detail: expected %q, got %v", "something went wrong", result["detail"])
	}
}

func TestProblemDetails_MarshalJSON_OmitsEmptyDetail(t *testing.T) {
	p := restapi.ProblemDetails{Type: "t", Title: "T", Status: 400}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := result["detail"]; ok {
		t.Error("detail should be omitted when empty")
	}
}

func TestProblemDetails_MarshalJSON_IncludesExtensions(t *testing.T) {
	p := restapi.ProblemDetails{
		Type:   "t",
		Title:  "T",
		Status: 422,
		Extensions: map[string]interface{}{
			"errors": map[string][]string{"field": {"required"}},
		},
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := result["errors"]; !ok {
		t.Error("expected 'errors' extension field in JSON output")
	}
}

func TestProblemDetails_MarshalJSON_ExtensionDoesNotOverrideStandardField(t *testing.T) {
	p := restapi.ProblemDetails{
		Type:   "t",
		Title:  "Original Title",
		Status: 400,
		Extensions: map[string]interface{}{
			"title": "Injected Title",
		},
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result["title"] != "Original Title" {
		t.Errorf("extension must not override standard field: title got %v", result["title"])
	}
}

func TestProblemDetails_UnmarshalJSON_ParsesStandardFields(t *testing.T) {
	raw := `{"type":"https://example.com/error","title":"Test","status":404,"detail":"not found","instance":"/api/v1/tags/123"}`

	var p restapi.ProblemDetails
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if p.Type != "https://example.com/error" {
		t.Errorf("Type: expected %q, got %q", "https://example.com/error", p.Type)
	}
	if p.Title != "Test" {
		t.Errorf("Title: expected %q, got %q", "Test", p.Title)
	}
	if p.Status != 404 {
		t.Errorf("Status: expected 404, got %d", p.Status)
	}
	if p.Detail != "not found" {
		t.Errorf("Detail: expected %q, got %q", "not found", p.Detail)
	}
	if p.Instance != "/api/v1/tags/123" {
		t.Errorf("Instance: expected %q, got %q", "/api/v1/tags/123", p.Instance)
	}
}

func TestProblemDetails_UnmarshalJSON_PopulatesExtensions(t *testing.T) {
	raw := `{"type":"t","title":"T","status":422,"errors":{"field":["required"]}}`

	var p restapi.ProblemDetails
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if p.Extensions == nil {
		t.Fatal("Extensions should not be nil")
	}
	if _, ok := p.Extensions["errors"]; !ok {
		t.Error("expected 'errors' in Extensions")
	}
}

func TestNewBadRequest_HasCorrectFields(t *testing.T) {
	p := restapi.NewBadRequest("bad input", "/api/v1/tags")

	if p.Status != http.StatusBadRequest {
		t.Errorf("Status: expected 400, got %d", p.Status)
	}
	if p.Type != restapi.TypeBadRequest {
		t.Errorf("Type: expected %q, got %q", restapi.TypeBadRequest, p.Type)
	}
	if p.Detail != "bad input" {
		t.Errorf("Detail: expected %q, got %q", "bad input", p.Detail)
	}
	if p.Instance != "/api/v1/tags" {
		t.Errorf("Instance: expected %q, got %q", "/api/v1/tags", p.Instance)
	}
}

func TestNewNotFound_HasCorrectStatusAndType(t *testing.T) {
	p := restapi.NewNotFound("tag", "abc-123", "/api/v1/tags/abc-123")

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
	p := restapi.NewConflict("tag", "already exists", "/api/v1/tags")

	if p.Status != http.StatusConflict {
		t.Errorf("Status: expected 409, got %d", p.Status)
	}
	if p.Type != restapi.TypeConflict {
		t.Errorf("Type: expected %q, got %q", restapi.TypeConflict, p.Type)
	}
}

func TestNewValidationError_HasExtensionsWithErrors(t *testing.T) {
	errs := map[string][]string{"name": {"required"}}
	p := restapi.NewValidationError(errs, "/api/v1/tags")

	if p.Status != http.StatusUnprocessableEntity {
		t.Errorf("Status: expected 422, got %d", p.Status)
	}
	if p.Type != restapi.TypeValidation {
		t.Errorf("Type: expected %q, got %q", restapi.TypeValidation, p.Type)
	}
	if p.Extensions == nil {
		t.Fatal("Extensions should not be nil")
	}
	if _, ok := p.Extensions["errors"]; !ok {
		t.Error("expected 'errors' in Extensions")
	}
}

func TestNewInternalError_HasCorrectFields(t *testing.T) {
	p := restapi.NewInternalError("database error", "/api/v1/tags")

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
