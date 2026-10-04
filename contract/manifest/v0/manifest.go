// Package manifestv0 is MAJOR 0 of the growctl manifest contract: the YAML
// documents an Engineer writes to declare Tags (ADR 0005).
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
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 1} }

// apiVersionPrefix precedes the version in apiVersion.
const apiVersionPrefix = "growscada/v"

// KindTag is the manifest kind for a Tag.
const KindTag = "Tag"

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

// Metadata identifies the declared entity.
type Metadata struct {
	Name string `yaml:"name"`
}

// TagSpec declares a Tag. InitialValue and InitialQuality are applied only
// when the Tag is created.
type TagSpec struct {
	Type           *string `yaml:"type" schema:"required"`
	InitialValue   *string `yaml:"initialValue" schema:"required"`
	InitialQuality *string `yaml:"initialQuality" schema:"required"`
}
