// Package projectv0 is MAJOR 0 of the project format: the ProjectFile and
// the Revision, which share one shape (ADR 0005). The format itself is not
// modelled yet; this package fixes its SchemaVersion and the root field that
// every project document carries.
package projectv0

import "github.com/kipitix/growscada/contract"

// SchemaVersion is the version of the project format this package describes.
// A function rather than a variable, so no importer can change it.
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 1} }

// Header is the root of every project document: the SchemaVersion the rest
// of the document follows. A reader checks it before anything else.
type Header struct {
	SchemaVersion contract.SchemaVersion `json:"schema_version"`
}
