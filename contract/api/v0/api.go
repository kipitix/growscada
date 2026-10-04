// Package apiv0 is MAJOR 0 of the server API contract: the JSON bodies of
// the REST API under PathPrefix and the live events of its event stream
// (ADR 0005).
//
// Readers of responses and events skip fields and event types they do not
// know; the server rejects request bodies with unknown fields. Changing any
// type here changes the contract: raise SchemaVersion and run make schemas.
package apiv0

import "github.com/kipitix/growscada/contract"

// SchemaVersion is the version of the server API this package describes.
// A function rather than a variable, so no importer can change it.
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 1} }

// PathPrefix is the URL path prefix of every endpoint of this MAJOR.
const PathPrefix = "/api/v0"
