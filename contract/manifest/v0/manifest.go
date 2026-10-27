// Package manifestv0 is MAJOR 0 of the growctl manifest contract: the YAML
// documents an Engineer writes to declare Projects and the Tags of a Project
// (ADR 0005, ADR 0006), Libraries with the WidgetTypes of their Drafts and
// the Library Pins of a Project (ADR 0007).
//
// A manifest names its version in apiVersion as "growscada/vMAJOR.MINOR";
// "growscada/vMAJOR" reads as MINOR 0. Manifests are written by people, so
// they are read strictly: unknown fields are errors, and a manifest of a
// newer MINOR than the reader knows is refused with a request to update the
// reader rather than an "unknown field" error.
package manifestv0

import (
	"fmt"
	"strings"

	"github.com/kipitix/growscada/contract"
)

// SchemaVersion is the version of the manifest contract this package
// describes.
// A function rather than a variable, so no importer can change it.
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 4} }

// apiVersionPrefix precedes the version in apiVersion.
const apiVersionPrefix = "growscada/v"

// Manifest kinds.
const (
	// KindTag is the manifest kind for a Tag.
	KindTag = "Tag"
	// KindProject is the manifest kind for a Project.
	KindProject = "Project"
	// KindLibrary is the manifest kind for a Library.
	KindLibrary = "Library"
	// KindWidgetType is the manifest kind for a WidgetType of a Library's
	// Draft.
	KindWidgetType = "WidgetType"
)

// APIVersion is the apiVersion written into manifests of this version.
func APIVersion() string {
	return FormatAPIVersion(SchemaVersion())
}

// FormatAPIVersion renders a version as an apiVersion value.
func FormatAPIVersion(v contract.SchemaVersion) string {
	return apiVersionPrefix + v.String()
}

// ParseAPIVersion parses "growscada/vMAJOR.MINOR" or "growscada/vMAJOR"
// (MINOR 0).
func ParseAPIVersion(s string) (contract.SchemaVersion, error) {
	version, ok := strings.CutPrefix(s, apiVersionPrefix)
	if !ok {
		return contract.SchemaVersion{}, fmt.Errorf("apiVersion %q: want %sMAJOR.MINOR", s, apiVersionPrefix)
	}
	if !strings.Contains(version, ".") {
		version += ".0"
	}
	v, err := contract.ParseSchemaVersion(version)
	if err != nil {
		return contract.SchemaVersion{}, fmt.Errorf("apiVersion %q: %w", s, err)
	}
	return v, nil
}

// Header is the kind-independent part of a manifest document.
type Header struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

// TagDocument is a `kind: Tag` document. Spec fields are pointers so that a
// missing field is told apart from an empty one; all of them are required.
type TagDocument struct {
	Header   `yaml:",inline"`
	Metadata Metadata `yaml:"metadata"`
	Spec     TagSpec  `yaml:"spec"`
}

// Metadata identifies the declared entity of a Project: its name, unique in
// the Project, and optionally the Project's name. Without Project the entity
// is of the Project growctl works in (its --project, the one the other
// manifests name, or its context); with one, it is of that Project, which
// --project, if given, must agree with.
type Metadata struct {
	Name    string `yaml:"name"`
	Project string `yaml:"project,omitempty"`
}

// ProjectDocument is a `kind: Project` document: a Project, identified by its
// name, unique on the server. Its spec is optional (since 0.4).
type ProjectDocument struct {
	Header   `yaml:",inline"`
	Metadata ProjectMetadata `yaml:"metadata"`
	Spec     *ProjectSpec    `yaml:"spec,omitempty"`
}

// ProjectSpec declares what a Project's Draft pins: LibraryPins maps the
// name of a Library to the number of its Library Release the Project uses
// (since 0.4). A Library the map leaves out keeps its pin, if any.
type ProjectSpec struct {
	LibraryPins map[string]int `yaml:"libraryPins,omitempty"`
}

// LibraryDocument is a `kind: Library` document (since 0.4): a Library,
// identified by its name, unique on the server. It has no spec: its
// WidgetTypes are documents of their own.
type LibraryDocument struct {
	Header   `yaml:",inline"`
	Metadata LibraryMetadata `yaml:"metadata"`
}

// LibraryMetadata identifies a declared Library.
type LibraryMetadata struct {
	Name string `yaml:"name"`
}

// WidgetTypeDocument is a `kind: WidgetType` document (since 0.4): a
// WidgetType of the Draft of the Library metadata.library, identified by its
// name, unique in that Library.
type WidgetTypeDocument struct {
	Header   `yaml:",inline"`
	Metadata WidgetTypeMetadata `yaml:"metadata"`
	Spec     WidgetTypeSpec     `yaml:"spec"`
}

// WidgetTypeMetadata identifies a declared WidgetType: its name and the name
// of its Library, both required.
type WidgetTypeMetadata struct {
	Name    string `yaml:"name"`
	Library string `yaml:"library"`
}

// WidgetTypeSpec declares a WidgetType's definition. HTML, Script, Width and
// Height are required; ScriptLanguage defaults to javascript, InputPorts to
// none.
type WidgetTypeSpec struct {
	HTML           *string     `yaml:"html" schema:"required"`
	Script         *string     `yaml:"script" schema:"required"`
	ScriptLanguage string      `yaml:"scriptLanguage,omitempty"`
	Width          *int        `yaml:"width" schema:"required"`
	Height         *int        `yaml:"height" schema:"required"`
	InputPorts     []InputPort `yaml:"inputPorts,omitempty"`
}

// InputPort declares an InputPort of a WidgetType. An empty TypeHint accepts
// any TagType.
type InputPort struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	TypeHint    string `yaml:"typeHint,omitempty"`
}

// ProjectMetadata identifies a declared Project.
type ProjectMetadata struct {
	Name string `yaml:"name"`
}

// TagSpec declares a Tag: its definition. Its state is no configuration: it
// starts as its type's zero value with quality bad until a supplier writes
// one (ADR 0011). Since 0.3 there is no initialValue nor initialQuality.
type TagSpec struct {
	Type *string `yaml:"type" schema:"required"`
}
