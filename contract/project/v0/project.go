// Package projectv0 is MAJOR 0 of the project format: the ProjectFile and
// the Deployment, which share one shape (ADR 0005). A ProjectFile is a full
// snapshot of a Project's configuration — no operational state, no history,
// no names of the Project itself (ADR 0006) — and a Deployment is that same
// document, numbered and stored by the Runtime node.
package projectv0

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/contract"
)

// SchemaVersion is the version of the project format this package describes.
// A function rather than a variable, so no importer can change it.
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 2} }

// Header is the root of every project document: the SchemaVersion the rest
// of the document follows, and the Project the document configures. A reader
// checks both before anything else.
//
// The Project is named by its identity alone: a Runtime node knows the name
// from its own records, and renaming a Project must not require a Deploy.
type Header struct {
	SchemaVersion contract.SchemaVersion `json:"schema_version"`
	ProjectID     uuid.UUID              `json:"project_id"`
}

// ProjectFile is a full snapshot of one Project's configuration, plus the
// content of the WidgetTypes it uses: the whole Draft as a Deploy delivers
// it. Every list is unordered — the order of a snapshot carries no meaning,
// and the builder that produces a ProjectFile canonicalizes it (sorted by
// ID, normalized JSON) so that the same configuration is always the same
// bytes.
//
// A ProjectFile holds no Tag states, no Device Tokens, no aggregate versions
// and no history: those are the Runtime node's, not configuration.
type ProjectFile struct {
	Header
	Tags               []TagDefinition     `json:"tags"`
	Devices            []Device            `json:"devices"`
	Environments       []Environment       `json:"environments"`
	EnvironmentDevices []EnvironmentDevice `json:"environment_devices"`
	Scenes             []Scene             `json:"scenes"`
	WidgetTypes        []WidgetType        `json:"widget_types"`
	LibraryPins        []LibraryPin        `json:"library_pins"`
}

// TagDefinition is a Tag as the Draft defines it: its identity, its name
// unique in the Project, its TagType and the Device that supplies its
// values. SupplierDeviceID is nil when people write the Tag.
type TagDefinition struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	Type             string     `json:"type"`
	SupplierDeviceID *uuid.UUID `json:"supplier_device_id,omitempty"`
}

// Device is a piece of equipment: what it is — its protocol and the
// configuration read under it — not where it stands. Config is the
// protocol-specific JSON object, carried as it is; a value that is not an
// object is invalid input. Comment is descriptive metadata and never affects
// behavior.
type Device struct {
	ID       uuid.UUID  `json:"id"`
	Name     string     `json:"name"`
	Comment  string     `json:"comment"`
	Protocol string     `json:"protocol"`
	Config   JSONObject `json:"config"`
}

// Environment is a named set of one Project's Devices as they stand in one
// context of operation. Comment is descriptive metadata. A Deploy does not
// choose an Environment; a Device Gateway resolves one by name.
type Environment struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Comment string    `json:"comment"`
}

// EnvironmentDevice is one Device as configured in one Environment: the pair
// (Environment, Device) and its connection parameters. The pair has no
// identity of its own: EnvironmentID and DeviceID name it. Params is the
// connection parameters as a JSON object, carried as it is; a value that is
// not an object is invalid input.
type EnvironmentDevice struct {
	EnvironmentID uuid.UUID  `json:"environment_id"`
	DeviceID      uuid.UUID  `json:"device_id"`
	Params        JSONObject `json:"params"`
}

// Scene is a mnemonic canvas of the Project with the Widgets placed on it.
type Scene struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	BackgroundHTML string    `json:"background_html"`
	Widgets        []Widget  `json:"widgets"`
}

// Widget is an instance of a WidgetType placed on a Scene: its geometry, the
// WidgetType it is of (TypeID names one of ProjectFile.WidgetTypes) and the
// Tags feeding its input ports. The transform matrix is derived from the
// geometry, so it is not part of the snapshot.
type Widget struct {
	ID           uuid.UUID     `json:"id"`
	Name         string        `json:"name"`
	Position     Position      `json:"position"`
	Size         Size          `json:"size"`
	Origin       Origin        `json:"origin"`
	Rotation     Rotation      `json:"rotation"`
	TypeID       uuid.UUID     `json:"type_id"`
	Labels       []string      `json:"labels"`
	PortBindings []PortBinding `json:"port_bindings"`
}

// Position is a Widget's place on its Scene: X and Y in the Scene's plane,
// Z in the stacking order.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z int     `json:"z"`
}

// Size is a Widget's dimensions.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Origin is a Widget's anchor point. X and Y are in [0, 1]: (0.5, 0.5) is
// the center.
type Origin struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Rotation is a Widget's rotation around its origin.
type Rotation struct {
	Degrees float64 `json:"degrees"`
}

// PortBinding links one of a Widget's input ports to the Tag that feeds it.
type PortBinding struct {
	PortName string    `json:"port_name"`
	TagID    uuid.UUID `json:"tag_id"`
}

// WidgetType is the copy of a WidgetType a Project uses, taken from the
// Library Release the Project pins: its template, its script and its input
// ports, with no version — a Release never changes, so a copy of it cannot
// either. A Widget's TypeID names one. The copy makes a Deployment, and a
// ProjectFile, independent of the Library.
type WidgetType struct {
	ID             uuid.UUID   `json:"id"`
	LibraryID      uuid.UUID   `json:"library_id"`
	Name           string      `json:"name"`
	HTMLTemplate   string      `json:"html_template"`
	Script         string      `json:"script"`
	ScriptLanguage string      `json:"script_language"`
	DefaultWidth   int         `json:"default_width"`
	DefaultHeight  int         `json:"default_height"`
	InputPorts     []InputPort `json:"input_ports"`
}

// InputPort is a WidgetType's input port. An empty TypeHint accepts any
// TagType.
type InputPort struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TypeHint    string `json:"type_hint"`
}

// LibraryPin is the Library Release a Project's Draft uses, by number: the
// provenance from which Discard restores the Project's pins.
type LibraryPin struct {
	LibraryID uuid.UUID `json:"library_id"`
	Release   int       `json:"release"`
}
