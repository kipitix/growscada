// Package apiv0 is MAJOR 0 of the server API contract: the JSON bodies of
// the REST API under PathPrefix and the live events of its event streams
// (ADR 0005). The content of a project (Tags, Scenes, Widgets) lives under
// ProjectPath of the project (ADR 0006); WidgetTypes and the projects
// themselves are the server's.
//
// Readers of responses and events skip fields and event types they do not
// know; the server rejects request bodies with unknown fields. Changing any
// type here changes the contract: raise SchemaVersion and run make schemas.
package apiv0

import "github.com/kipitix/growscada/contract"

// SchemaVersion is the version of the server API this package describes.
// A function rather than a variable, so no importer can change it.
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 5} }

// PathPrefix is the URL path prefix of every endpoint of this MAJOR.
const PathPrefix = "/api/v0"

// ProjectPath is the path, under PathPrefix, of the project with the given
// ID: the prefix of the resources of its content and of its event stream.
func ProjectPath(projectID string) string { return "/projects/" + projectID }

// EventsQueryProject is the parameter of GET /events joining to the stream
// the events of the project with this ID. A project not found adds nothing:
// the stream carries the server's events alone.
const EventsQueryProject = "project"
