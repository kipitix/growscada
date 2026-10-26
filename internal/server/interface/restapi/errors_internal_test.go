package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
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
			name:       "project not found of a tag's path names the project",
			err:        wrapped(id.NotFoundError{Err: project.ErrProjectNotFound, ID: uuid.MustParse(errorsTestID)}),
			pathParams: map[string]string{"project_id": errorsTestID, "tag_id": "11111111-1111-1111-1111-111111111111"},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "Project with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "project conflict",
			err:        wrapped(project.ErrProjectConflict),
			pathParams: map[string]string{"project_id": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: Project",
			wantDetail: "Project was changed concurrently; reload it and retry",
		},
		{
			name:       "project name taken",
			err:        wrapped(project.ErrProjectNameTaken),
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: Project",
			wantDetail: "Project name is already taken",
		},
		{
			name:       "tag not found",
			err:        wrapped(tag.ErrTagNotFound),
			pathParams: map[string]string{"tag_id": errorsTestID},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "Tag with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "tag conflict",
			err:        wrapped(tag.ErrTagConflict),
			pathParams: map[string]string{"tag_id": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: Tag",
			wantDetail: "Tag was changed concurrently; reload it and retry",
		},
		{
			name:       "tag name taken",
			err:        wrapped(tag.ErrTagNameTaken),
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: Tag",
			wantDetail: "Tag name is already taken in the project",
		},
		{
			name:       "widget type not found",
			err:        wrapped(library.ErrWidgetTypeNotFound),
			pathParams: map[string]string{"widget_type_id": errorsTestID},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "WidgetType with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "widget type conflict",
			err:        wrapped(library.ErrWidgetTypeConflict),
			pathParams: map[string]string{"widget_type_id": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: WidgetType",
			wantDetail: "WidgetType was changed concurrently; reload it and retry",
		},
		{
			name:       "widget type in use by deleted projects only",
			err:        wrapped(library.ErrWidgetTypeInUseByDeletedProjects),
			pathParams: map[string]string{"widget_type_id": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: WidgetType",
			wantDetail: "WidgetType is used only by Widgets of deleted Projects, still being removed; retry shortly",
		},
		{
			name:       "widget type in use",
			err:        wrapped(library.ErrWidgetTypeInUse),
			pathParams: map[string]string{"widget_type_id": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: WidgetType",
			wantDetail: "WidgetType is used by Widgets",
		},
		{
			name:       "scene not found",
			err:        wrapped(scene.ErrSceneNotFound),
			pathParams: map[string]string{"scene_id": errorsTestID},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "Scene with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "scene conflict",
			err:        wrapped(scene.ErrSceneConflict),
			pathParams: map[string]string{"scene_id": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: Scene",
			wantDetail: "Scene was changed concurrently; reload it and retry",
		},
		{
			name:       "scene race lost on a request without a version",
			err:        fmt.Errorf("%w: %w", application.ErrRaceLost, wrapped(scene.ErrSceneConflict)),
			pathParams: map[string]string{"scene_id": errorsTestID},
			wantStatus: http.StatusConflict,
			wantTitle:  "Conflict: Scene",
			wantDetail: "Scene kept changing while the request ran; retry it later",
		},
		{
			name:       "widget not found names the widget, not the scene",
			err:        wrapped(scene.ErrWidgetNotFound),
			pathParams: map[string]string{"scene_id": "11111111-1111-1111-1111-111111111111", "widget_id": errorsTestID},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "Widget with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "not found naming the path's ID is 404",
			err:        wrapped(id.NotFoundError{Err: scene.ErrSceneNotFound, ID: uuid.MustParse(errorsTestID)}),
			pathParams: map[string]string{"scene_id": errorsTestID},
			wantStatus: http.StatusNotFound,
			wantTitle:  "Resource Not Found",
			wantDetail: "Scene with ID '" + errorsTestID + "' not found",
		},
		{
			name:       "not found naming another ID than the path's is 500",
			err:        wrapped(id.NotFoundError{Err: scene.ErrSceneNotFound, ID: uuid.MustParse("11111111-1111-1111-1111-111111111111")}),
			pathParams: map[string]string{"scene_id": errorsTestID},
			wantStatus: http.StatusInternalServerError,
			wantTitle:  "Internal Server Error",
			wantDetail: "an internal error occurred",
		},
		{
			name:       "invalid input over a not-found reference from the body is 400",
			err:        invalidInputErr{err: fmt.Errorf("type_id: %w", library.ErrWidgetTypeNotFound)},
			pathParams: map[string]string{"scene_id": errorsTestID},
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Bad Request",
			wantDetail: "type_id: widget type not found",
		},
		{
			name:       "not found for a resource the path does not name is 500",
			err:        wrapped(library.ErrWidgetTypeNotFound),
			pathParams: map[string]string{"scene_id": errorsTestID},
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
			assertProblem(t, tt.err, tt.pathParams, tt.wantStatus, tt.wantTitle, tt.wantDetail)
		})
	}
}

func TestSendError_MappingWithUnhandledStatus_Is500(t *testing.T) {
	sentinel := errors.New("gone")
	saved := errorMappings
	errorMappings = append([]errorMapping{{sentinel, http.StatusGone, tagResource, "gone"}}, saved...)
	t.Cleanup(func() { errorMappings = saved })

	assertProblem(t, wrapped(sentinel), nil, http.StatusInternalServerError,
		"Internal Server Error", "an internal error occurred")
}

// assertProblem runs sendError on err and checks the Problem Details it sends.
func assertProblem(t *testing.T, err error, pathParams map[string]string, wantStatus int, wantTitle, wantDetail string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v0/anything", nil)
	for k, v := range pathParams {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()

	sendError(rec, req, err)

	if rec.Code != wantStatus {
		t.Errorf("status: expected %d, got %d", wantStatus, rec.Code)
	}
	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Status != wantStatus {
		t.Errorf("problem status: expected %d, got %d", wantStatus, prob.Status)
	}
	if prob.Title != wantTitle {
		t.Errorf("title: expected %q, got %q", wantTitle, prob.Title)
	}
	if prob.Detail != wantDetail {
		t.Errorf("detail: expected %q, got %q", wantDetail, prob.Detail)
	}
	if strings.Contains(prob.Detail, "internal wrapper text") {
		t.Errorf("detail %q leaks the wrapping error text", prob.Detail)
	}
}

// domainSentinels lists every exported Err… variable of the domain packages;
// TestDomainSentinels_AreListed keeps it complete.
var domainSentinels = map[string]error{
	"project.ErrProjectNotFound":                  project.ErrProjectNotFound,
	"project.ErrProjectConflict":                  project.ErrProjectConflict,
	"project.ErrProjectNameTaken":                 project.ErrProjectNameTaken,
	"project.ErrProjectNotEmpty":                  project.ErrProjectNotEmpty,
	"library.ErrWidgetTypeNotFound":               library.ErrWidgetTypeNotFound,
	"library.ErrWidgetTypeConflict":               library.ErrWidgetTypeConflict,
	"library.ErrWidgetTypeInUse":                  library.ErrWidgetTypeInUse,
	"library.ErrWidgetTypeInUseByDeletedProjects": library.ErrWidgetTypeInUseByDeletedProjects,
	"library.ErrInvalidWidgetType":                library.ErrInvalidWidgetType,
	"library.ErrDuplicateInputPortName":           library.ErrDuplicateInputPortName,
	"scene.ErrSceneNotFound":                      scene.ErrSceneNotFound,
	"scene.ErrSceneConflict":                      scene.ErrSceneConflict,
	"scene.ErrWidgetNotFound":                     scene.ErrWidgetNotFound,
	"scene.ErrWidgetAlreadyExists":                scene.ErrWidgetAlreadyExists,
	"scene.ErrWidgetTypeMismatch":                 scene.ErrWidgetTypeMismatch,
	"scene.ErrPortNotDeclared":                    scene.ErrPortNotDeclared,
	"scene.ErrBoundTagNotFound":                   scene.ErrBoundTagNotFound,
	"scene.ErrPortTypeMismatch":                   scene.ErrPortTypeMismatch,
	"scene.ErrTagTypesOfAnotherProject":           scene.ErrTagTypesOfAnotherProject,
	"tag.ErrTagNotFound":                          tag.ErrTagNotFound,
	"tag.ErrTagConflict":                          tag.ErrTagConflict,
	"tag.ErrTagNameTaken":                         tag.ErrTagNameTaken,
	"tag.ErrInvalidTagNameMatcher":                tag.ErrInvalidTagNameMatcher,
}

// unmappedSentinels are the domain sentinels errorMappings leaves out on
// purpose, with the answer they get instead.
var unmappedSentinels = map[string]string{
	"library.ErrInvalidWidgetType":      "400: the service marks it invalid input",
	"library.ErrDuplicateInputPortName": "400: comes inside ErrInvalidWidgetType or a create, marked invalid input",
	"scene.ErrPortNotDeclared":          "400: the service marks it invalid input (port_bindings)",
	"scene.ErrBoundTagNotFound":         "400: the service marks it invalid input (port_bindings)",
	"scene.ErrPortTypeMismatch":         "400: the service marks it invalid input (port_bindings)",
	"tag.ErrInvalidTagNameMatcher":      "400: the service marks it invalid input",
	"scene.ErrWidgetAlreadyExists":      "500: the server makes a new Widget's ID, a clash is its fault",
	"scene.ErrWidgetTypeMismatch":       "500: the service checks a Widget against its own WidgetType",
	"project.ErrProjectNotEmpty":        "500: only the cleanup policy removes a project's row, never the API",
	"scene.ErrTagTypesOfAnotherProject": "500: the service reads the TagTypes through the Scene's own Project",
}

// TestDomainSentinels_AreListed reads the domain sources, so a new sentinel
// fails here until it is listed in domainSentinels and then either mapped or
// listed in unmappedSentinels.
func TestDomainSentinels_AreListed(t *testing.T) {
	const domainDir = "../../domain"
	fset := token.NewFileSet()
	found := map[string]bool{}
	err := filepath.WalkDir(domainDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					if strings.HasPrefix(name.Name, "Err") && name.IsExported() {
						found[f.Name.Name+"."+name.Name] = true
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read domain sources: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no sentinels found under " + domainDir)
	}

	for name := range found {
		if _, ok := domainSentinels[name]; !ok {
			t.Errorf("domain sentinel %s is not in domainSentinels", name)
		}
	}
	for name := range domainSentinels {
		if !found[name] {
			t.Errorf("domainSentinels lists %s, which the domain no longer declares", name)
		}
	}
}

func TestErrorMappings_CoverEveryDomainSentinelOnce(t *testing.T) {
	mapped := map[error]int{}
	for _, m := range errorMappings {
		mapped[m.sentinel]++
		if m.status == http.StatusConflict && m.detail == "" {
			t.Errorf("409 for %q has no detail", m.sentinel)
		}
	}
	for name, sentinel := range domainSentinels {
		_, unmapped := unmappedSentinels[name]
		switch n := mapped[sentinel]; {
		case n > 1:
			t.Errorf("%s mapped %d times", name, n)
		case n == 1 && unmapped:
			t.Errorf("%s is both mapped and in unmappedSentinels", name)
		case n == 0 && !unmapped:
			t.Errorf("%s is neither mapped nor in unmappedSentinels", name)
		}
	}
	for name := range unmappedSentinels {
		if _, ok := domainSentinels[name]; !ok {
			t.Errorf("unmappedSentinels lists %s, which is no domain sentinel", name)
		}
	}
}

func TestPathUUID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, apiv0.PathPrefix+"/projects/"+errorsTestID+"/scenes/abc", nil)
	req.SetPathValue("scene_id", "abc")
	rec := httptest.NewRecorder()

	if _, ok := pathUUID(rec, req, sceneResource); ok {
		t.Fatal("pathUUID accepted 'abc'")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: expected 400, got %d", rec.Code)
	}
	var prob apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if want := "scene_id: 'abc' is not a valid UUID"; prob.Detail != want {
		t.Errorf("detail: expected %q, got %q", want, prob.Detail)
	}

	req.SetPathValue("scene_id", errorsTestID)
	id, ok := pathUUID(httptest.NewRecorder(), req, sceneResource)
	if !ok || id.String() != errorsTestID {
		t.Errorf("pathUUID(%q) = %v, %v", errorsTestID, id, ok)
	}
}
