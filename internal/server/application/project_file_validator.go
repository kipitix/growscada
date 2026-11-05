package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/contract"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// FileIssue is one problem of a project file: where in the document it is and
// what is wrong there (ADR 0018). Path is the document path, e.g.
// "schema_version", "library_pins[0]" or "scenes[2].widgets[0].type_id".
type FileIssue struct {
	Path    string
	Message string
}

// ProjectFileError is a project file that does not validate: every problem
// found, not the first (ADR 0018). It matches ErrInvalidProjectFile.
type ProjectFileError struct {
	Issues []FileIssue
}

func (e ProjectFileError) Error() string {
	if len(e.Issues) == 1 {
		return "invalid project file: " + e.Issues[0].Message
	}
	return fmt.Sprintf("invalid project file: %d problems", len(e.Issues))
}

func (e ProjectFileError) Is(target error) bool { return target == ErrInvalidProjectFile }

// projectFileValidation is a document that validates: its canonical form, the
// Draft content it would make and the WidgetType copies the content uses.
type projectFileValidation struct {
	file    projectv0.ProjectFile
	content draft.Content
}

// projectFileValidator validates a ProjectFile for the Engineering node
// (ADR 0018). Opening also checks the document's pins against this server's
// live Libraries and their releases; deploying a file does not, because the
// document carries the WidgetTypes its Widgets use.
type projectFileValidator struct {
	libraries library.LibraryRepository
	releases  library.LibraryReleaseRepository
}

// validate decodes document for projectID, checks its schema version, its
// Project, its invariants and cross-references, rebuilds the ProjectFile it
// would make and compares the two canonical forms. checkLibraries also
// requires every pin to name a live Library with that Release here and every
// copy to match it.
func (v projectFileValidator) validate(ctx context.Context, projectID id.ID[project.Project], document []byte, checkLibraries bool) (projectFileValidation, error) {
	// The schema version is read before the strict decode: a file of a newer
	// MINOR may carry fields this build does not know, and "newer than the
	// server" is the honest answer to it, not an unknown-field error.
	header, headerIssues := decodeProjectFileHeader(document)
	if len(headerIssues) > 0 {
		return projectFileValidation{}, ProjectFileError{Issues: headerIssues}
	}
	var problems fileProblems
	checkSchemaVersion(header.SchemaVersion, &problems)
	if len(problems.issues) > 0 {
		return projectFileValidation{}, ProjectFileError{Issues: problems.issues}
	}

	file, decodeIssues := decodeProjectFile(document)
	if len(decodeIssues) > 0 {
		return projectFileValidation{}, ProjectFileError{Issues: decodeIssues}
	}
	if file.ProjectID != projectID.UUID() {
		problems.add("project_id", "the file configures project %s, not %s", file.ProjectID, projectID)
	}
	shape := validateProjectFileShape(file, &problems)
	if len(problems.issues) > 0 {
		return projectFileValidation{}, ProjectFileError{Issues: problems.issues}
	}

	canonical := canonicalProjectFile(file)
	content, err := contentOfRevision(projectID, canonical)
	if err != nil {
		problems.add("", "the document cannot be read as a Draft: %s", err)
		return projectFileValidation{}, ProjectFileError{Issues: problems.issues}
	}

	copies := shape.types
	if checkLibraries {
		copies, err = v.localWidgetTypes(ctx, canonical, &problems)
		if err != nil {
			return projectFileValidation{}, err
		}
		if len(problems.issues) > 0 {
			return projectFileValidation{}, ProjectFileError{Issues: problems.issues}
		}
	}

	rebuilt, err := projectFileOfContent(projectID, content, func(typeID id.ID[library.WidgetType]) (projectv0.WidgetType, error) {
		copied, ok := copies[typeID.UUID()]
		if !ok {
			return projectv0.WidgetType{}, fmt.Errorf("%w: %s", library.ErrWidgetTypeNotFound, typeID)
		}
		return copied, nil
	})
	if err != nil {
		problems.add("", "the document does not describe a deployable project: %s", err)
		return projectFileValidation{}, ProjectFileError{Issues: problems.issues}
	}
	same, err := sameProjectFile(rebuilt, canonical)
	if err != nil {
		return projectFileValidation{}, err
	}
	if !same {
		problems.add("", "the document differs from the Draft it would make: an element is missing, extra, or does not match its content")
		return projectFileValidation{}, ProjectFileError{Issues: problems.issues}
	}
	return projectFileValidation{file: canonical, content: content}, nil
}

// decodeProjectFileHeader reads the document's header leniently, for the
// schema version that decides the strict decode: a field of a newer MINOR is
// not an error yet.
func decodeProjectFileHeader(document []byte) (projectv0.Header, []FileIssue) {
	var header projectv0.Header
	if err := json.Unmarshal(document, &header); err != nil {
		return header, []FileIssue{{Path: "", Message: err.Error()}}
	}
	return header, nil
}

// decodeProjectFile decodes document strictly (ADR 0005): a field this build
// does not know is a problem, not something to skip.
func decodeProjectFile(document []byte) (projectv0.ProjectFile, []FileIssue) {
	var file projectv0.ProjectFile
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return file, []FileIssue{{Path: unknownFieldPath(err.Error()), Message: err.Error()}}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return file, []FileIssue{{Path: "", Message: "the document has data after the project file"}}
	}
	return file, nil
}

// unknownFieldPath names the field a strict decode refused, when the decoder
// says which one.
func unknownFieldPath(message string) string {
	const prefix = `json: unknown field "`
	index := strings.Index(message, prefix)
	if index < 0 {
		return ""
	}
	rest := message[index+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// checkSchemaVersion applies ADR 0018's rule: the same MAJOR, MINOR not newer
// than the server's. A newer file is refused with its own message, which is
// clearer than the unknown-field error of decoding a newer document.
func checkSchemaVersion(fileVersion contract.SchemaVersion, problems *fileProblems) {
	server := projectv0.SchemaVersion()
	switch {
	case fileVersion.NewerThan(server):
		problems.add("schema_version", "the file's schema %s is newer than this server's %s: the file is newer than the server", fileVersion, server)
	case !fileVersion.SameMajor(server):
		problems.add("schema_version", "the file's schema %s does not match this server's %s", fileVersion, server)
	}
}

// fileProblems collects the problems of one document, in the order they are
// found.
type fileProblems struct {
	issues []FileIssue
}

func (p *fileProblems) add(path, format string, args ...any) {
	p.issues = append(p.issues, FileIssue{Path: path, Message: fmt.Sprintf(format, args...)})
}

// projectFileShape is what the cross-references of a document resolve to.
type projectFileShape struct {
	tags         map[uuid.UUID]projectv0.TagDefinition
	devices      map[uuid.UUID]projectv0.Device
	environments map[uuid.UUID]projectv0.Environment
	types        map[uuid.UUID]projectv0.WidgetType
	pins         map[uuid.UUID]projectv0.LibraryPin
}

// validateProjectFileShape enforces the invariants and the cross-references
// of a document without a database (ADR 0018): valid names, types, protocols,
// sizes and languages; unique IDs and natural keys; every reference names an
// element of the same document; every Widget belongs to exactly one Scene.
func validateProjectFileShape(file projectv0.ProjectFile, problems *fileProblems) projectFileShape {
	shape := projectFileShape{
		tags:         make(map[uuid.UUID]projectv0.TagDefinition, len(file.Tags)),
		devices:      make(map[uuid.UUID]projectv0.Device, len(file.Devices)),
		environments: make(map[uuid.UUID]projectv0.Environment, len(file.Environments)),
		types:        make(map[uuid.UUID]projectv0.WidgetType, len(file.WidgetTypes)),
		pins:         make(map[uuid.UUID]projectv0.LibraryPin, len(file.LibraryPins)),
	}

	deviceNames := make(map[string]struct{}, len(file.Devices))
	for i, d := range file.Devices {
		path := fmt.Sprintf("devices[%d]", i)
		checkID(d.ID, path+".id", problems)
		if _, ok := shape.devices[d.ID]; ok && d.ID != uuid.Nil {
			problems.add(path+".id", "the ID %s appears more than once", d.ID)
		}
		shape.devices[d.ID] = d
		if _, err := device.NewDeviceName(d.Name); err != nil {
			problems.add(path+".name", "%s", err)
		}
		if _, ok := deviceNames[d.Name]; ok && d.Name != "" {
			problems.add(path+".name", "the name %q appears more than once", d.Name)
		}
		deviceNames[d.Name] = struct{}{}
		if _, err := device.NewDeviceProtocol(d.Protocol); err != nil {
			problems.add(path+".protocol", "%s", err)
		}
		if _, err := device.NewDeviceConfig([]byte(d.Config)); err != nil {
			problems.add(path+".config", "%s", err)
		}
	}

	environmentNames := make(map[string]struct{}, len(file.Environments))
	for i, e := range file.Environments {
		path := fmt.Sprintf("environments[%d]", i)
		checkID(e.ID, path+".id", problems)
		if _, ok := shape.environments[e.ID]; ok && e.ID != uuid.Nil {
			problems.add(path+".id", "the ID %s appears more than once", e.ID)
		}
		shape.environments[e.ID] = e
		if _, err := device.NewEnvironmentName(e.Name); err != nil {
			problems.add(path+".name", "%s", err)
		}
		if _, ok := environmentNames[e.Name]; ok && e.Name != "" {
			problems.add(path+".name", "the name %q appears more than once", e.Name)
		}
		environmentNames[e.Name] = struct{}{}
	}

	tagNames := make(map[string]struct{}, len(file.Tags))
	for i, t := range file.Tags {
		path := fmt.Sprintf("tags[%d]", i)
		checkID(t.ID, path+".id", problems)
		if _, ok := shape.tags[t.ID]; ok && t.ID != uuid.Nil {
			problems.add(path+".id", "the ID %s appears more than once", t.ID)
		}
		shape.tags[t.ID] = t
		if _, err := tag.NewTagName(t.Name); err != nil {
			problems.add(path+".name", "%s", err)
		}
		if _, ok := tagNames[t.Name]; ok && t.Name != "" {
			problems.add(path+".name", "the name %q appears more than once", t.Name)
		}
		tagNames[t.Name] = struct{}{}
		if _, err := tag.NewTagType(t.Type); err != nil {
			problems.add(path+".type", "%s", err)
		}
	}

	pairs := make(map[string]struct{}, len(file.EnvironmentDevices))
	for i, p := range file.EnvironmentDevices {
		path := fmt.Sprintf("environment_devices[%d]", i)
		if _, ok := shape.environments[p.EnvironmentID]; !ok {
			problems.add(path+".environment_id", "environment %s is not in the file", p.EnvironmentID)
		}
		if _, ok := shape.devices[p.DeviceID]; !ok {
			problems.add(path+".device_id", "device %s is not in the file", p.DeviceID)
		}
		key := p.EnvironmentID.String() + "/" + p.DeviceID.String()
		if _, ok := pairs[key]; ok {
			problems.add(path, "the pair of environment %s and device %s appears more than once", p.EnvironmentID, p.DeviceID)
		}
		pairs[key] = struct{}{}
		if _, err := device.NewEnvironmentDeviceParams([]byte(p.Params)); err != nil {
			problems.add(path+".params", "%s", err)
		}
	}

	for i, t := range file.Tags {
		if t.SupplierDeviceID == nil {
			continue
		}
		if _, ok := shape.devices[*t.SupplierDeviceID]; !ok {
			problems.add(fmt.Sprintf("tags[%d].supplier_device_id", i),
				"device %s is not in the file", *t.SupplierDeviceID)
		}
	}

	typeNames := make(map[string]struct{}, len(file.WidgetTypes))
	for i, wt := range file.WidgetTypes {
		path := fmt.Sprintf("widget_types[%d]", i)
		checkID(wt.ID, path+".id", problems)
		if _, ok := shape.types[wt.ID]; ok && wt.ID != uuid.Nil {
			problems.add(path+".id", "the ID %s appears more than once", wt.ID)
		}
		shape.types[wt.ID] = wt
		checkID(wt.LibraryID, path+".library_id", problems)
		if _, err := library.NewWidgetTypeName(wt.Name); err != nil {
			problems.add(path+".name", "%s", err)
		}
		nameKey := wt.LibraryID.String() + "/" + wt.Name
		if _, ok := typeNames[nameKey]; ok && wt.Name != "" {
			problems.add(path+".name", "the name %q appears more than once in library %s", wt.Name, wt.LibraryID)
		}
		typeNames[nameKey] = struct{}{}
		if _, err := library.NewHtmlTemplate(wt.HTMLTemplate); err != nil {
			problems.add(path+".html_template", "%s", err)
		}
		if _, err := library.NewScript(wt.Script); err != nil {
			problems.add(path+".script", "%s", err)
		}
		if _, err := library.NewScriptLanguage(wt.ScriptLanguage); err != nil {
			problems.add(path+".script_language", "%s", err)
		}
		if _, err := library.NewSize(wt.DefaultWidth, wt.DefaultHeight); err != nil {
			problems.add(path+".default_width", "%s", err)
		}
		portNames := make(map[string]struct{}, len(wt.InputPorts))
		for j, port := range wt.InputPorts {
			portPath := fmt.Sprintf("%s.input_ports[%d]", path, j)
			if _, err := library.NewInputPortName(port.Name); err != nil {
				problems.add(portPath+".name", "%s", err)
			}
			if _, ok := portNames[port.Name]; ok && port.Name != "" {
				problems.add(portPath+".name", "the port name %q appears more than once", port.Name)
			}
			portNames[port.Name] = struct{}{}
			if _, err := library.NewPortTypeHint(port.TypeHint); err != nil {
				problems.add(portPath+".type_hint", "%s", err)
			}
		}
	}

	for i, pin := range file.LibraryPins {
		path := fmt.Sprintf("library_pins[%d]", i)
		checkID(pin.LibraryID, path+".library_id", problems)
		if _, ok := shape.pins[pin.LibraryID]; ok && pin.LibraryID != uuid.Nil {
			problems.add(path+".library_id", "the library %s appears more than once", pin.LibraryID)
		}
		shape.pins[pin.LibraryID] = pin
		if _, err := library.NewReleaseNumber(pin.Release); err != nil {
			problems.add(path+".release", "%s", err)
		}
	}

	sceneNames := make(map[string]struct{}, len(file.Scenes))
	widgets := make(map[uuid.UUID]string, len(file.Scenes))
	for i, sc := range file.Scenes {
		path := fmt.Sprintf("scenes[%d]", i)
		checkID(sc.ID, path+".id", problems)
		if _, err := scene.NewSceneName(sc.Name); err != nil {
			problems.add(path+".name", "%s", err)
		}
		if _, ok := sceneNames[sc.Name]; ok && sc.Name != "" {
			problems.add(path+".name", "the name %q appears more than once", sc.Name)
		}
		sceneNames[sc.Name] = struct{}{}
		if _, err := scene.NewSceneSize(sc.Width, sc.Height); err != nil {
			problems.add(path+".width", "%s", err)
		}
		for j, w := range sc.Widgets {
			widgetPath := fmt.Sprintf("%s.widgets[%d]", path, j)
			checkID(w.ID, widgetPath+".id", problems)
			if owner, ok := widgets[w.ID]; ok && w.ID != uuid.Nil {
				problems.add(widgetPath+".id", "the widget also belongs to %s: a widget belongs to exactly one scene", owner)
			}
			widgets[w.ID] = path
			if _, err := scene.NewWidgetName(w.Name); err != nil {
				problems.add(widgetPath+".name", "%s", err)
			}
			if _, err := library.NewSize(w.Size.Width, w.Size.Height); err != nil {
				problems.add(widgetPath+".size.width", "%s", err)
			}
			if _, err := scene.NewOrigin(w.Origin.X, w.Origin.Y); err != nil {
				problems.add(widgetPath+".origin", "%s", err)
			}
			copied, ok := shape.types[w.TypeID]
			if !ok {
				problems.add(widgetPath+".type_id", "widget type %s is not in widget_types", w.TypeID)
			} else if _, pinned := shape.pins[copied.LibraryID]; !pinned {
				problems.add(widgetPath+".type_id",
					"widget type %s is of library %s, which library_pins does not name", w.TypeID, copied.LibraryID)
			}
			portNames := make(map[string]struct{}, len(w.PortBindings))
			for k, binding := range w.PortBindings {
				bindingPath := fmt.Sprintf("%s.port_bindings[%d]", widgetPath, k)
				if _, err := library.NewInputPortName(binding.PortName); err != nil {
					problems.add(bindingPath+".port_name", "%s", err)
				}
				if _, ok := portNames[binding.PortName]; ok && binding.PortName != "" {
					problems.add(bindingPath+".port_name", "the port name %q is bound more than once", binding.PortName)
				}
				portNames[binding.PortName] = struct{}{}
				if _, ok := shape.tags[binding.TagID]; !ok {
					problems.add(bindingPath+".tag_id", "tag %s is not in the file", binding.TagID)
				}
			}
		}
	}
	return shape
}

func checkID(value uuid.UUID, path string, problems *fileProblems) {
	if value == uuid.Nil {
		problems.add(path, "an entity needs an ID")
	}
}

// localWidgetTypes checks a document that opens against this server's
// Libraries (ADR 0018): every pin names a live Library with that Release, and
// every copy the document carries matches the Release's copy. It returns the
// copies the rebuild uses, by WidgetType ID.
func (v projectFileValidator) localWidgetTypes(ctx context.Context, file projectv0.ProjectFile, problems *fileProblems) (map[uuid.UUID]projectv0.WidgetType, error) {
	types := make(map[uuid.UUID]projectv0.WidgetType)
	for i, pin := range file.LibraryPins {
		path := fmt.Sprintf("library_pins[%d]", i)
		libraryID := id.NewID(id.IDWithUUID[library.Library](pin.LibraryID))
		lib, err := v.libraries.FindByID(ctx, libraryID)
		if errors.Is(err, library.ErrLibraryNotFound) {
			problems.add(path, "library %s is not live on this server", pin.LibraryID)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read library %s: %w", pin.LibraryID, err)
		}
		number, err := library.NewReleaseNumber(pin.Release)
		if err != nil {
			problems.add(path+".release", "%s", err)
			continue
		}
		release, err := v.releases.FindByNumber(ctx, lib.ID(), number)
		if errors.Is(err, library.ErrReleaseNotFound) {
			problems.add(path, "library %s has no release %d on this server", pin.LibraryID, pin.Release)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read release %d of library %s: %w", pin.Release, pin.LibraryID, err)
		}
		for _, released := range release.WidgetTypes() {
			types[released.ID().UUID()] = newProjectFileWidgetType(released)
		}
	}
	for i, copied := range file.WidgetTypes {
		local, ok := types[copied.ID]
		if !ok {
			// A copy no pinned release holds: if a Widget uses it, the
			// rebuild below names the Widget; an unused one is extra.
			continue
		}
		if !sameWidgetTypeCopy(local, copied) {
			problems.add(fmt.Sprintf("widget_types[%d]", i),
				"the copy of widget type %s differs from the release of library %s pinned here", copied.ID, copied.LibraryID)
		}
	}
	return types, nil
}

// sameWidgetTypeCopy reports whether two copies of a WidgetType are the same
// in identity and content.
func sameWidgetTypeCopy(a, b projectv0.WidgetType) bool {
	left, err := json.Marshal(canonicalWidgetType(a))
	if err != nil {
		return false
	}
	right, err := json.Marshal(canonicalWidgetType(b))
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}

func canonicalWidgetType(wt projectv0.WidgetType) projectv0.WidgetType {
	wt.InputPorts = cloneOrEmpty(wt.InputPorts)
	return wt
}
