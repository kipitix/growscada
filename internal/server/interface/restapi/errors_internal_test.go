package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

const errorsTestID = "0b6c7b9e-4a4e-4a43-9d1b-2f7f0a8a3c11"

// invalidInputErr stands for an error a service marked as invalid input.
type invalidInputErr struct{ err error }

func (e invalidInputErr) Error() string        { return e.err.Error() }
func (e invalidInputErr) Unwrap() error        { return e.err }
func (e invalidInputErr) Is(target error) bool { return target == application.ErrInvalidInput }

// wrapped wraps err the way services and repositories do, so the test sees
// whether the wrapping text leaks into the response.
func wrapped(err error) error {
	return fmt.Errorf("internal wrapper text: %w", err)
}

func TestSendError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		pathParams map[string]string
		wantStatus int
		wantTitle  string
		wantDetail string
	}{
		{
			name:       "tag not found",
			err:        wrapped(tag.ErrTagNotFound),
			pathParams: map[string]string{"tagId": errorsTestID},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "Tag with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "tag conflict",
			err:        wrapped(tag.ErrTagConflict),
			pathParams: map[string]string{"tagId": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: Tag",
			wantDetail: "Tag was changed concurrently; reload it and retry",
		},
		{
			name:       "tag name taken",
			err:        wrapped(tag.ErrTagNameTaken),
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: Tag",
			wantDetail: "Tag name is already taken",
		},
		{
			name:       "widget type not found",
			err:        wrapped(library.ErrWidgetTypeNotFound),
			pathParams: map[string]string{"widgetTypeId": errorsTestID},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "WidgetType with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "widget type conflict",
			err:        wrapped(library.ErrWidgetTypeConflict),
			pathParams: map[string]string{"widgetTypeId": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: WidgetType",
			wantDetail: "WidgetType was changed concurrently; reload it and retry",
		},
		{
			name:       "widget type in use",
			err:        wrapped(library.ErrWidgetTypeInUse),
			pathParams: map[string]string{"widgetTypeId": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: WidgetType",
			wantDetail: "WidgetType is used by Widgets",
		},
		{
			name:       "scene not found",
			err:        wrapped(scene.ErrSceneNotFound),
			pathParams: map[string]string{"sceneId": errorsTestID},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "Scene with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "scene conflict",
			err:        wrapped(scene.ErrSceneConflict),
			pathParams: map[string]string{"sceneId": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: Scene",
			wantDetail: "Scene was changed concurrently; reload it and retry",
		},
		{
			name:       "widget not found names the widget, not the scene",
			err:        wrapped(scene.ErrWidgetNotFound),
			pathParams: map[string]string{"sceneId": "11111111-1111-1111-1111-111111111111", "widgetId": errorsTestID},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "Widget with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "invalid input over a not-found reference from the body is 400",
			err:        invalidInputErr{err: fmt.Errorf("type_id: %w", library.ErrWidgetTypeNotFound)},
			pathParams: map[string]string{"sceneId": errorsTestID},
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Bad Request",
			wantDetail: "type_id: widget type not found",
		},
		{
			name:       "not found for a resource the path does not name is 500",
			err:        wrapped(library.ErrWidgetTypeNotFound),
			pathParams: map[string]string{"sceneId": errorsTestID},
			wantStatus: http.StatusInternalServerError,
			wantTitle:  "Internal Server Error",
			wantDetail: "an internal error occurred",
		},
		{
			name:       "unknown error is 500 without its text",
			err:        wrapped(errors.New("connection refused")),
			wantStatus: http.StatusInternalServerError,
			wantTitle:  "Internal Server Error",
			wantDetail: "an internal error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v0/anything", nil)
			for k, v := range tt.pathParams {
				req.SetPathValue(k, v)
			}
			rec := httptest.NewRecorder()

			sendError(rec, req, tt.err)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: expected %d, got %d", tt.wantStatus, rec.Code)
			}
			var prob ProblemDetails
			if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if prob.Status != tt.wantStatus {
				t.Errorf("problem status: expected %d, got %d", tt.wantStatus, prob.Status)
			}
			if prob.Title != tt.wantTitle {
				t.Errorf("title: expected %q, got %q", tt.wantTitle, prob.Title)
			}
			if prob.Detail != tt.wantDetail {
				t.Errorf("detail: expected %q, got %q", tt.wantDetail, prob.Detail)
			}
			if strings.Contains(prob.Detail, "internal wrapper text") {
				t.Errorf("detail %q leaks the wrapping error text", prob.Detail)
			}
		})
	}
}

func TestErrorMappings_CoverEverySentinelOnce(t *testing.T) {
	seen := map[error]bool{}
	for _, m := range errorMappings {
		if seen[m.sentinel] {
			t.Errorf("sentinel %q mapped twice", m.sentinel)
		}
		seen[m.sentinel] = true
		if m.status == http.StatusConflict && m.detail == "" {
			t.Errorf("409 for %q has no detail", m.sentinel)
		}
	}
}

func TestPathUUID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v0/scenes/abc", nil)
	req.SetPathValue("sceneId", "abc")
	rec := httptest.NewRecorder()

	if _, ok := pathUUID(rec, req, sceneResource); ok {
		t.Fatal("pathUUID accepted 'abc'")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
	var prob ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if want := "sceneId: 'abc' is not a valid UUID"; prob.Detail != want {
		t.Errorf("detail: expected %q, got %q", want, prob.Detail)
	}

	req.SetPathValue("sceneId", errorsTestID)
	id, ok := pathUUID(httptest.NewRecorder(), req, sceneResource)
	if !ok || id.String() != errorsTestID {
		t.Errorf("pathUUID(%q) = %v, %v", errorsTestID, id, ok)
	}
}
