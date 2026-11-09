package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/description"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// emptyFile is a valid document of a Project with no content.
func emptyFile(projectID uuid.UUID) projectv0.ProjectFile {
	return projectv0.ProjectFile{
		Header:             projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID},
		Tags:               []projectv0.TagDefinition{},
		Devices:            []projectv0.Device{},
		Environments:       []projectv0.Environment{},
		EnvironmentDevices: []projectv0.EnvironmentDevice{},
		Scenes:             []projectv0.Scene{},
		WidgetTypes:        []projectv0.WidgetType{},
		LibraryPins:        []projectv0.LibraryPin{},
	}
}

func encoded(t *testing.T, file projectv0.ProjectFile) []byte {
	t.Helper()
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("encode the document: %v", err)
	}
	return data
}

// validateFile validates a document as a deploy of a file does: the local
// Libraries are not checked.
func validateFile(t *testing.T, projectID uuid.UUID, file projectv0.ProjectFile) error {
	t.Helper()
	_, err := (projectFileValidator{}).validate(context.Background(),
		id.NewID(id.IDWithUUID[project.Project](projectID)), encoded(t, file), false)
	return err
}

// fileIssues returns the issues of a refused document, failing on any other
// outcome.
func fileIssues(t *testing.T, err error) []FileIssue {
	t.Helper()
	var invalid ProjectFileError
	if !errors.As(err, &invalid) {
		t.Fatalf("error %v, want a ProjectFileError", err)
	}
	if !errors.Is(err, ErrInvalidProjectFile) {
		t.Errorf("the error does not match ErrInvalidProjectFile")
	}
	return invalid.Issues
}

func wantIssue(t *testing.T, issues []FileIssue, path, contains string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Path == path && strings.Contains(issue.Message, contains) {
			return
		}
	}
	t.Fatalf("no issue at %q containing %q: %+v", path, contains, issues)
}

func TestValidateProjectFile_AcceptsACanonicalEmptyDocument(t *testing.T) {
	projectID := uuid.New()
	if err := validateFile(t, projectID, emptyFile(projectID)); err != nil {
		t.Fatalf("a valid empty document: %v", err)
	}
}

func TestValidateProjectFile_RefusesANewerSchema(t *testing.T) {
	projectID := uuid.New()
	file := emptyFile(projectID)
	file.SchemaVersion.Minor = projectv0.SchemaVersion().Minor + 1
	issues := fileIssues(t, validateFile(t, projectID, file))
	wantIssue(t, issues, "schema_version", "newer")
}

// TestValidateProjectFile_RefusesAnotherMajor: a MAJOR this build does not
// read is newer than it and is refused as such.
func TestValidateProjectFile_RefusesAnotherMajor(t *testing.T) {
	projectID := uuid.New()
	file := emptyFile(projectID)
	file.SchemaVersion.Major = projectv0.SchemaVersion().Major + 1
	issues := fileIssues(t, validateFile(t, projectID, file))
	wantIssue(t, issues, "schema_version", "newer")
}

// TestValidateProjectFile_RefusesANewerSchemaBeforeUnknownFields: a newer
// MINOR is answered as such even when it carries a field this build cannot
// decode (ADR 0018).
func TestValidateProjectFile_RefusesANewerSchemaBeforeUnknownFields(t *testing.T) {
	projectID := uuid.New()
	document := []byte(`{"schema_version":"0.` + fmt.Sprint(projectv0.SchemaVersion().Minor+1) + `","project_id":"` +
		projectID.String() + `","surprise":true}`)
	_, err := (projectFileValidator{}).validate(context.Background(),
		id.NewID(id.IDWithUUID[project.Project](projectID)), document, false)
	issues := fileIssues(t, err)
	wantIssue(t, issues, "schema_version", "newer")
}

func TestValidateProjectFile_RefusesAnotherProject(t *testing.T) {
	file := emptyFile(uuid.New())
	issues := fileIssues(t, validateFile(t, uuid.New(), file))
	wantIssue(t, issues, "project_id", "configures project")
}

func TestValidateProjectFile_RefusesAnUnknownField(t *testing.T) {
	projectID := uuid.New()
	document := []byte(`{"schema_version":"` + projectv0.SchemaVersion().String() + `","project_id":"` +
		projectID.String() + `","tags":[],"devices":[],"environments":[],"environment_devices":[],"scenes":[],"widget_types":[],"library_pins":[],"surprise":true}`)
	_, err := (projectFileValidator{}).validate(context.Background(),
		id.NewID(id.IDWithUUID[project.Project](projectID)), document, false)
	issues := fileIssues(t, err)
	wantIssue(t, issues, "surprise", "unknown field")
}

func TestValidateProjectFile_RefusesDuplicateIDsAndNames(t *testing.T) {
	projectID := uuid.New()
	file := emptyFile(projectID)
	tagID := uuid.New()
	file.Tags = []projectv0.TagDefinition{
		{ID: tagID, Name: "speed", Type: "integer"},
		{ID: tagID, Name: "speed", Type: "integer"},
	}
	issues := fileIssues(t, validateFile(t, projectID, file))
	wantIssue(t, issues, "tags[1].id", "appears more than once")
	wantIssue(t, issues, "tags[1].name", "appears more than once")
}

// TestValidateProjectFile_RefusesTwoScenesOfOneID: a Scene's ID names one
// Scene in the Project; a second Scene of it would be written over the first
// instead of being refused (ADR 0018).
func TestValidateProjectFile_RefusesTwoScenesOfOneID(t *testing.T) {
	projectID := uuid.New()
	file := emptyFile(projectID)
	sceneID := uuid.New()
	file.Scenes = []projectv0.Scene{
		{ID: sceneID, Name: "One", Width: 100, Height: 100},
		{ID: sceneID, Name: "Two", Width: 100, Height: 100},
	}
	issues := fileIssues(t, validateFile(t, projectID, file))
	wantIssue(t, issues, "scenes[1].id", "appears more than once")
}

// portFile is a document of one Scene with one Widget of a WidgetType that
// declares ports, the Widget binding boundPort to a Tag of tagType.
func portFile(t *testing.T, projectID uuid.UUID, tagType string, ports []projectv0.InputPort, boundPort string) projectv0.ProjectFile {
	t.Helper()
	libraryID, typeID, tagID := uuid.New(), uuid.New(), uuid.New()
	file := emptyFile(projectID)
	file.Tags = []projectv0.TagDefinition{{ID: tagID, Name: "speed", Type: tagType}}
	file.LibraryPins = []projectv0.LibraryPin{{LibraryID: libraryID, Release: 1}}
	file.WidgetTypes = []projectv0.WidgetType{{
		ID: typeID, LibraryID: libraryID, Name: "gauge", ScriptLanguage: "javascript",
		DefaultWidth: 10, DefaultHeight: 10, InputPorts: ports,
	}}
	file.Scenes = []projectv0.Scene{{
		ID: uuid.New(), Name: "Main", Width: 100, Height: 100,
		Widgets: []projectv0.Widget{{
			ID: uuid.New(), Name: "gauge", Size: projectv0.Size{Width: 10, Height: 10},
			Origin: projectv0.Origin{X: 0.5, Y: 0.5}, TypeID: typeID,
			PortBindings: []projectv0.PortBinding{{PortName: boundPort, TagID: tagID}},
		}},
	}}
	return file
}

// TestValidateProjectFile_RefusesAPortItsWidgetTypeDoesNotDeclare: the domain
// accepts a binding only to a port the WidgetType declares (ADR 0018).
func TestValidateProjectFile_RefusesAPortItsWidgetTypeDoesNotDeclare(t *testing.T) {
	projectID := uuid.New()
	issues := fileIssues(t, validateFile(t, projectID, portFile(t, projectID, "integer", nil, "missing")))
	wantIssue(t, issues, "scenes[0].widgets[0].port_bindings[0].port_name", "does not declare")
}

// TestValidateProjectFile_RefusesABindingOfTheWrongTagType: a port's hint must
// accept the bound Tag's TagType, as every Widget write checks (ADR 0018).
func TestValidateProjectFile_RefusesABindingOfTheWrongTagType(t *testing.T) {
	projectID := uuid.New()
	file := portFile(t, projectID, "boolean",
		[]projectv0.InputPort{{Name: "value", TypeHint: "integer"}}, "value")
	issues := fileIssues(t, validateFile(t, projectID, file))
	wantIssue(t, issues, "scenes[0].widgets[0].port_bindings[0].tag_id", "accepts integer")
}

// TestValidateProjectFile_AcceptsADeclaredPortOfTheRightType is the control of
// the two refusals above.
func TestValidateProjectFile_AcceptsADeclaredPortOfTheRightType(t *testing.T) {
	projectID := uuid.New()
	file := portFile(t, projectID, "integer",
		[]projectv0.InputPort{{Name: "value", TypeHint: "integer"}}, "value")
	if err := validateFile(t, projectID, file); err != nil {
		t.Fatalf("a declared port with a matching tag: %v", err)
	}
}

func TestValidateProjectFile_RefusesDanglingReferences(t *testing.T) {
	projectID := uuid.New()
	file := emptyFile(projectID)
	deviceID, tagID := uuid.New(), uuid.New()
	file.Tags = []projectv0.TagDefinition{{ID: tagID, Name: "speed", Type: "integer", SupplierDeviceID: &deviceID}}
	file.Devices = []projectv0.Device{{ID: uuid.New(), Name: "meter", Protocol: "simulation", Config: projectv0.JSONObject(`{}`)}}
	file.Environments = []projectv0.Environment{{ID: uuid.New(), Name: "bench"}}
	file.EnvironmentDevices = []projectv0.EnvironmentDevice{{EnvironmentID: uuid.New(), DeviceID: uuid.New(), Params: projectv0.JSONObject(`{}`)}}
	file.Scenes = []projectv0.Scene{{
		ID: uuid.New(), Name: "Main", Width: 100, Height: 100,
		Widgets: []projectv0.Widget{{
			ID: uuid.New(), Name: "pressure", Size: projectv0.Size{Width: 10, Height: 10},
			Origin: projectv0.Origin{X: 0.5, Y: 0.5}, TypeID: uuid.New(),
			PortBindings: []projectv0.PortBinding{{PortName: "value", TagID: uuid.New()}},
		}},
	}}
	issues := fileIssues(t, validateFile(t, projectID, file))
	wantIssue(t, issues, "tags[0].supplier_device_id", "not in the file")
	wantIssue(t, issues, "environment_devices[0].environment_id", "not in the file")
	wantIssue(t, issues, "environment_devices[0].device_id", "not in the file")
	wantIssue(t, issues, "scenes[0].widgets[0].type_id", "not in widget_types")
	wantIssue(t, issues, "scenes[0].widgets[0].port_bindings[0].tag_id", "not in the file")
}

func TestValidateProjectFile_RefusesAWidgetOfTwoScenes(t *testing.T) {
	projectID := uuid.New()
	file := emptyFile(projectID)
	widgetID, typeID, tagID, libraryID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	file.Tags = []projectv0.TagDefinition{{ID: tagID, Name: "speed", Type: "integer"}}
	file.WidgetTypes = []projectv0.WidgetType{{
		ID: typeID, LibraryID: libraryID, Name: "gauge", ScriptLanguage: "javascript",
		DefaultWidth: 10, DefaultHeight: 10,
	}}
	file.LibraryPins = []projectv0.LibraryPin{{LibraryID: libraryID, Release: 1}}
	widget := projectv0.Widget{
		ID: widgetID, Name: "gauge", Size: projectv0.Size{Width: 10, Height: 10},
		Origin: projectv0.Origin{X: 0.5, Y: 0.5}, TypeID: typeID,
		PortBindings: []projectv0.PortBinding{{PortName: "value", TagID: tagID}},
	}
	file.Scenes = []projectv0.Scene{
		{ID: uuid.New(), Name: "One", Width: 100, Height: 100, Widgets: []projectv0.Widget{widget}},
		{ID: uuid.New(), Name: "Two", Width: 100, Height: 100, Widgets: []projectv0.Widget{widget}},
	}
	issues := fileIssues(t, validateFile(t, projectID, file))
	wantIssue(t, issues, "scenes[1].widgets[0].id", "belongs to exactly one scene")
}

func TestValidateProjectFile_RefusesAnExtraWidgetTypeCopy(t *testing.T) {
	projectID := uuid.New()
	file := emptyFile(projectID)
	file.WidgetTypes = []projectv0.WidgetType{{
		ID: uuid.New(), LibraryID: uuid.New(), Name: "unused", ScriptLanguage: "javascript",
		DefaultWidth: 10, DefaultHeight: 10,
	}}
	issues := fileIssues(t, validateFile(t, projectID, file))
	wantIssue(t, issues, "", "differs from the Draft it would make")
}

// TestValidateProjectFile_RefusesADocumentItsOwnRebuildDiffersFrom: a value
// the domain normalizes (a rotation of 370° is 10°) makes the document differ
// from the Draft it would make, and the file is refused (ADR 0018).
func TestValidateProjectFile_RefusesADocumentItsOwnRebuildDiffersFrom(t *testing.T) {
	projectID, libraryID := uuid.New(), uuid.New()
	file, _ := pinnedFile(t, projectID, libraryID)
	file.Scenes[0].Widgets[0].Rotation.Degrees = 370
	issues := fileIssues(t, validateFile(t, projectID, file))
	wantIssue(t, issues, "", "differs from the Draft it would make")
}

// fakeLibraries serves FindByID from a map; the other methods are not called.
type fakeLibraries struct {
	library.LibraryRepository
	found map[uuid.UUID]library.Library
}

func (f fakeLibraries) FindByID(_ context.Context, libraryID id.ID[library.Library]) (library.Library, error) {
	found, ok := f.found[libraryID.UUID()]
	if !ok {
		return nil, id.NotFound(library.ErrLibraryNotFound, libraryID)
	}
	return found, nil
}

// fakeReleases serves FindByNumber from a map, as fakeLibraries does.
type fakeReleases struct {
	library.LibraryReleaseRepository
	found map[string]library.LibraryRelease
}

func (f fakeReleases) FindByNumber(_ context.Context, libraryID id.ID[library.Library], number library.ReleaseNumber) (library.LibraryRelease, error) {
	found, ok := f.found[libraryID.UUID().String()+"/"+number.String()]
	if !ok {
		return nil, id.NotFound(library.ErrReleaseNotFound, libraryID)
	}
	return found, nil
}

// pinnedFile is a document of one Scene with one Widget of a pinned release.
func pinnedFile(t *testing.T, projectID, libraryID uuid.UUID) (projectv0.ProjectFile, library.LibraryRelease) {
	t.Helper()
	typeID, tagID := uuid.New(), uuid.New()
	name, err := library.NewWidgetTypeName("gauge")
	if err != nil {
		t.Fatalf("widget type name: %v", err)
	}
	html, err := library.NewHtmlTemplate("")
	if err != nil {
		t.Fatalf("html template: %v", err)
	}
	script, err := library.NewScript("")
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	portName, err := library.NewInputPortName("value")
	if err != nil {
		t.Fatalf("port name: %v", err)
	}
	released, err := library.NewReleasedWidgetType(
		id.NewID(id.IDWithUUID[library.WidgetType](typeID)),
		id.NewID(id.IDWithUUID[library.Library](libraryID)),
		name, html, script, library.ScriptLanguageJavaScript, library.DefaultSize(),
		[]library.InputPort{library.NewInputPort(portName, "", library.AnyTypeHint())},
	)
	if err != nil {
		t.Fatalf("released widget type: %v", err)
	}
	number, err := library.NewReleaseNumber(1)
	if err != nil {
		t.Fatalf("release number: %v", err)
	}
	author, err := library.NewAuthor("system")
	if err != nil {
		t.Fatalf("author: %v", err)
	}
	release, err := library.ReconstituteRelease(
		id.NewID(id.IDWithUUID[library.Library](libraryID)), number, nil, author, library.OriginLegacy, "",
		time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC), []library.ReleasedWidgetType{released})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	file := emptyFile(projectID)
	file.Tags = []projectv0.TagDefinition{{ID: tagID, Name: "speed", Type: "integer"}}
	file.LibraryPins = []projectv0.LibraryPin{{LibraryID: libraryID, Release: 1}}
	file.WidgetTypes = []projectv0.WidgetType{{
		ID: typeID, LibraryID: libraryID, Name: "gauge", ScriptLanguage: "javascript",
		DefaultWidth: 100, DefaultHeight: 100,
		InputPorts: []projectv0.InputPort{{Name: "value"}},
	}}
	file.Scenes = []projectv0.Scene{{
		ID: uuid.New(), Name: "Main", Width: 100, Height: 100,
		Widgets: []projectv0.Widget{{
			ID: uuid.New(), Name: "gauge", Size: projectv0.Size{Width: 100, Height: 100},
			Origin: projectv0.Origin{X: 0.5, Y: 0.5}, TypeID: typeID,
			PortBindings: []projectv0.PortBinding{{PortName: "value", TagID: tagID}},
		}},
	}}
	return file, release
}

func TestValidateProjectFile_OpenChecksLocalLibraries(t *testing.T) {
	projectID, libraryID := uuid.New(), uuid.New()
	file, release := pinnedFile(t, projectID, libraryID)
	libraryName, err := library.NewLibraryName("lib")
	if err != nil {
		t.Fatalf("library name: %v", err)
	}
	found := library.ReconstituteLibrary(
		id.NewID(id.IDWithUUID[library.Library](libraryID)),
		id.NewID[organization.Organization](),
		libraryName, description.Description{}, false, version.Initial[library.Library](),
	)
	validator := projectFileValidator{
		libraries: fakeLibraries{found: map[uuid.UUID]library.Library{libraryID: found}},
		releases:  fakeReleases{found: map[string]library.LibraryRelease{libraryID.String() + "/1": release}},
	}
	if _, err := validator.validate(context.Background(),
		id.NewID(id.IDWithUUID[project.Project](projectID)), encoded(t, file), true); err != nil {
		t.Fatalf("a locally backed document: %v", err)
	}

	// A Library this server does not have.
	validator.libraries = fakeLibraries{}
	issues := fileIssues(t, func() error {
		_, err := validator.validate(context.Background(),
			id.NewID(id.IDWithUUID[project.Project](projectID)), encoded(t, file), true)
		return err
	}())
	wantIssue(t, issues, "library_pins[0]", "is not live")

	// A Release this server does not have.
	validator.libraries = fakeLibraries{found: map[uuid.UUID]library.Library{libraryID: found}}
	validator.releases = fakeReleases{}
	issues = fileIssues(t, func() error {
		_, err := validator.validate(context.Background(),
			id.NewID(id.IDWithUUID[project.Project](projectID)), encoded(t, file), true)
		return err
	}())
	wantIssue(t, issues, "library_pins[0]", "has no release")

	// A copy that differs from the released WidgetType.
	validator.releases = fakeReleases{found: map[string]library.LibraryRelease{libraryID.String() + "/1": release}}
	file.WidgetTypes[0].HTMLTemplate = "changed"
	issues = fileIssues(t, func() error {
		_, err := validator.validate(context.Background(),
			id.NewID(id.IDWithUUID[project.Project](projectID)), encoded(t, file), true)
		return err
	}())
	wantIssue(t, issues, "widget_types[0]", "differs from the release")
}

// TestValidateProjectFile_DeployDoesNotCheckLocalLibraries: a self-contained
// document runs on a Runtime node that has none of its Libraries (ADR 0018).
func TestValidateProjectFile_DeployDoesNotCheckLocalLibraries(t *testing.T) {
	projectID, libraryID := uuid.New(), uuid.New()
	file, _ := pinnedFile(t, projectID, libraryID)
	validator := projectFileValidator{libraries: fakeLibraries{}, releases: fakeReleases{}}
	if _, err := validator.validate(context.Background(),
		id.NewID(id.IDWithUUID[project.Project](projectID)), encoded(t, file), false); err != nil {
		t.Fatalf("a valid self-contained document: %v", err)
	}
}
