// Package contract holds GrowSCADA's versioned contracts — the shapes of the
// data that crosses a process or time boundary (ADR 0005). Each contract has
// its own SchemaVersion and lives in its own subpackage per MAJOR:
//
//   - api/vN      — the server API: REST requests, responses and live events;
//   - project/vN  — the project format: ProjectFile and Revision;
//   - record/vN   — the operational record format: Journal entries,
//     Checkpoints and PlaybackFiles;
//   - manifest/vN — growctl manifests.
//
// The package is public so that tools and Device adapters outside this
// repository can share the exact shapes the server speaks. JSON schemas of
// every published version are generated from these types into
// schemas/<contract>/<MAJOR.MINOR>.json (make schemas).
package contract

import (
	"fmt"
	"strconv"
	"strings"
)

// SchemaVersionHeader is the HTTP header carrying the server API's full
// SchemaVersion: the server sets it on every response, clients send theirs
// on every request. The MAJOR is also in the URL path.
const SchemaVersionHeader = "GrowSCADA-Schema-Version"

// SchemaVersion is the MAJOR.MINOR version of a contract's schema. MINOR
// grows when old readers can safely ignore what was added; MAJOR grows on
// anything else. While MAJOR is 0 every change bumps only MINOR and no
// compatibility is promised.
//
// It marshals as the text "MAJOR.MINOR", so it can be a field of any
// stored document.
type SchemaVersion struct {
	Major int
	Minor int
}

// ParseSchemaVersion parses "MAJOR.MINOR". A bare "MAJOR" is rejected:
// whether it means MAJOR.0 depends on the contract (manifests allow it).
func ParseSchemaVersion(s string) (SchemaVersion, error) {
	majorText, minorText, ok := strings.Cut(s, ".")
	if !ok {
		return SchemaVersion{}, fmt.Errorf("schema version %q: want MAJOR.MINOR", s)
	}
	major, err := parseVersionNumber(majorText)
	if err != nil {
		return SchemaVersion{}, fmt.Errorf("schema version %q: invalid MAJOR: %w", s, err)
	}
	minor, err := parseVersionNumber(minorText)
	if err != nil {
		return SchemaVersion{}, fmt.Errorf("schema version %q: invalid MINOR: %w", s, err)
	}
	return SchemaVersion{Major: major, Minor: minor}, nil
}

// parseVersionNumber accepts a non-negative decimal without sign or leading
// zeros, so every version has exactly one spelling.
func parseVersionNumber(s string) (int, error) {
	if s == "" || (len(s) > 1 && s[0] == '0') || strings.ContainsAny(s, "+-") {
		return 0, fmt.Errorf("%q is not a version number", s)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a version number", s)
	}
	return n, nil
}

// String renders the version as "MAJOR.MINOR".
func (v SchemaVersion) String() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor)
}

// SameMajor reports whether both versions belong to the same schema, i.e.
// whether a reader of one can read the other at all.
func (v SchemaVersion) SameMajor(other SchemaVersion) bool {
	return v.Major == other.Major
}

// NewerThan reports whether v is a later version than other.
func (v SchemaVersion) NewerThan(other SchemaVersion) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	return v.Minor > other.Minor
}

// MarshalText implements encoding.TextMarshaler.
func (v SchemaVersion) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (v *SchemaVersion) UnmarshalText(text []byte) error {
	parsed, err := ParseSchemaVersion(string(text))
	if err != nil {
		return err
	}
	*v = parsed
	return nil
}

// JSONSchema describes the text form for the schema generator.
func (SchemaVersion) JSONSchema() map[string]any {
	return map[string]any{
		"type":    "string",
		"pattern": `^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`,
	}
}
