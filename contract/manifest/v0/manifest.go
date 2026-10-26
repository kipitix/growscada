// Package manifestv0 is MAJOR 0 of the growctl manifest contract: the YAML
// documents an Engineer writes to declare Projects and the Tags of a Project
// (ADR 0005, ADR 0006).
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
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 3} }

// apiVersionPrefix precedes the version in apiVersion.
const apiVersionPrefix = "growscada/v"

// Manifest kinds.
const (
	// KindTag is the manifest kind for a Tag.
	KindTag = "Tag"
	// KindProject is the manifest kind for a Project.
	KindProject = "Project"
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
// name, unique on the server. It has no spec yet.
type ProjectDocument struct {
	Header   `yaml:",inline"`
	Metadata ProjectMetadata `yaml:"metadata"`
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
