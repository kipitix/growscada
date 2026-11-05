package draft

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// Kind names the kind of a Draft entity a Snapshot carries. It is the
// discriminator of both the snapshot log and the write of a DraftChange.
type Kind string

const (
	KindTag               Kind = "tag"
	KindScene             Kind = "scene"
	KindWidget            Kind = "widget"
	KindDevice            Kind = "device"
	KindEnvironment       Kind = "environment"
	KindEnvironmentDevice Kind = "environment_device"
	KindLibraryPin        Kind = "library_pin"
	KindWidgetType        Kind = "widget_type"
)

// Snapshot is the full stored state of one entity of a Draft: its kind, its
// ID and its row. A change that created or removed an entity carries one side
// only (ADR 0017).
//
// A Scene's Widgets are snapshots of their own: a widget row is what changes
// when a binding is removed, and the Scene row stays as it is. A pair
// (Environment, Device) has no ID of its own: its ID is the Environment's and
// its state names the Device. A Library Pin's ID is its Library's.
type Snapshot struct {
	Kind  Kind
	ID    uuid.UUID
	State State
}

// State is the typed stored state of an entity a Snapshot carries.
type State interface {
	// kind is the Kind the state belongs to; it keeps a state of one kind
	// out of a snapshot of another.
	kind() Kind
}

// TagState is a Tag definition's row.
type TagState struct {
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Supplier *uuid.UUID `json:"supplier_device_id,omitempty"`
}

func (TagState) kind() Kind { return KindTag }

// SceneState is a Scene's row, its widgets excluded.
type SceneState struct {
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
}

func (SceneState) kind() Kind { return KindScene }

// WidgetState is a Widget's row.
type WidgetState struct {
	SceneID         uuid.UUID          `json:"scene_id"`
	Name            string             `json:"name"`
	X               float64            `json:"x"`
	Y               float64            `json:"y"`
	Z               int                `json:"z"`
	Width           int                `json:"width"`
	Height          int                `json:"height"`
	OriginX         float64            `json:"origin_x"`
	OriginY         float64            `json:"origin_y"`
	RotationDegrees float64            `json:"rotation_degrees"`
	TypeID          uuid.UUID          `json:"type_id"`
	Labels          []string           `json:"labels"`
	PortBindings    []PortBindingState `json:"port_bindings"`
}

func (WidgetState) kind() Kind { return KindWidget }

// PortBindingState is one PortBinding of a Widget's row.
type PortBindingState struct {
	PortName string    `json:"port_name"`
	TagID    uuid.UUID `json:"tag_id"`
}

// DeviceState is a Device's row.
type DeviceState struct {
	Name     string          `json:"name"`
	Comment  string          `json:"comment"`
	Protocol string          `json:"protocol"`
	Config   json.RawMessage `json:"config"`
}

func (DeviceState) kind() Kind { return KindDevice }

// EnvironmentState is an Environment's row.
type EnvironmentState struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

func (EnvironmentState) kind() Kind { return KindEnvironment }

// EnvironmentDeviceState is a pair's row; its ID is the Environment's.
type EnvironmentDeviceState struct {
	EnvironmentID uuid.UUID       `json:"environment_id"`
	DeviceID      uuid.UUID       `json:"device_id"`
	Params        json.RawMessage `json:"params"`
}

func (EnvironmentDeviceState) kind() Kind { return KindEnvironmentDevice }

// WidgetTypeState is a WidgetType's row of a Library's Draft.
type WidgetTypeState struct {
	LibraryID      uuid.UUID        `json:"library_id"`
	Name           string           `json:"name"`
	HtmlTemplate   string           `json:"html_template"`
	Script         string           `json:"script"`
	ScriptLanguage string           `json:"script_language"`
	DefaultWidth   int              `json:"default_width"`
	DefaultHeight  int              `json:"default_height"`
	InputPorts     []InputPortState `json:"input_ports"`
}

func (WidgetTypeState) kind() Kind { return KindWidgetType }

// InputPortState is one InputPort of a WidgetType's row.
type InputPortState struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TypeHint    string `json:"type_hint"`
}

// LibraryPinState is a pin's row; its ID is the Library's.
type LibraryPinState struct {
	LibraryID uuid.UUID `json:"library_id"`
	Release   int       `json:"release"`
}

func (LibraryPinState) kind() Kind { return KindLibraryPin }

// SnapshotOfTag is the snapshot of a Tag definition.
func SnapshotOfTag(aTag tag.Tag) Snapshot {
	state := TagState{Name: aTag.Name().String(), Type: aTag.Type().String()}
	if supplier := aTag.Supplier(); supplier.HasDevice() {
		deviceID := supplier.DeviceID().UUID()
		state.Supplier = &deviceID
	}
	return Snapshot{Kind: KindTag, ID: aTag.ID().UUID(), State: state}
}

// SnapshotOfScene is the snapshot of a Scene's own row.
func SnapshotOfScene(aScene scene.Scene) Snapshot {
	return Snapshot{Kind: KindScene, ID: aScene.ID().UUID(), State: SceneState{
		Name:           aScene.Name().String(),
		Width:          aScene.Size().Width(),
		Height:         aScene.Size().Height(),
		BackgroundHTML: aScene.BackgroundHTML().Content(),
	}}
}

// SnapshotOfWidget is the snapshot of a Widget of the Scene sceneID.
func SnapshotOfWidget(sceneID id.ID[scene.Scene], w scene.Widget) Snapshot {
	bindings := w.PortBindings()
	state := WidgetState{
		SceneID:         sceneID.UUID(),
		Name:            w.Name().String(),
		X:               w.Position().X(),
		Y:               w.Position().Y(),
		Z:               w.Position().Z(),
		Width:           w.Size().Width(),
		Height:          w.Size().Height(),
		OriginX:         w.Origin().X(),
		OriginY:         w.Origin().Y(),
		RotationDegrees: w.Rotation().Degrees(),
		TypeID:          w.TypeID().UUID(),
		Labels:          w.Labels(),
		PortBindings:    make([]PortBindingState, len(bindings)),
	}
	for i, b := range bindings {
		state.PortBindings[i] = PortBindingState{PortName: b.PortName().String(), TagID: b.TagID().UUID()}
	}
	return Snapshot{Kind: KindWidget, ID: w.ID().UUID(), State: state}
}

// SnapshotOfDevice is the snapshot of a Device.
func SnapshotOfDevice(d device.Device) Snapshot {
	return Snapshot{Kind: KindDevice, ID: d.ID().UUID(), State: DeviceState{
		Name: d.Name().String(), Comment: d.Comment(),
		Protocol: d.Protocol().String(), Config: json.RawMessage(d.Config().Bytes()),
	}}
}

// SnapshotOfEnvironment is the snapshot of an Environment.
func SnapshotOfEnvironment(e device.Environment) Snapshot {
	return Snapshot{Kind: KindEnvironment, ID: e.ID().UUID(), State: EnvironmentState{
		Name: e.Name().String(), Comment: e.Comment(),
	}}
}

// SnapshotOfEnvironmentDevice is the snapshot of a pair; its ID is the
// Environment's.
func SnapshotOfEnvironmentDevice(p device.EnvironmentDevice) Snapshot {
	return Snapshot{Kind: KindEnvironmentDevice, ID: p.EnvironmentID().UUID(), State: EnvironmentDeviceState{
		EnvironmentID: p.EnvironmentID().UUID(),
		DeviceID:      p.DeviceID().UUID(),
		Params:        json.RawMessage(p.Params().Bytes()),
	}}
}

// SnapshotOfWidgetType is the snapshot of a WidgetType of a Library's Draft.
func SnapshotOfWidgetType(wt library.WidgetType) Snapshot {
	ports := wt.InputPorts()
	state := WidgetTypeState{
		LibraryID:      wt.LibraryID().UUID(),
		Name:           wt.Name().String(),
		HtmlTemplate:   wt.HtmlTemplate().String(),
		Script:         wt.Script().String(),
		ScriptLanguage: wt.ScriptLanguage().String(),
		DefaultWidth:   wt.DefaultSize().Width(),
		DefaultHeight:  wt.DefaultSize().Height(),
		InputPorts:     make([]InputPortState, len(ports)),
	}
	for i, p := range ports {
		state.InputPorts[i] = InputPortState{
			Name: p.Name().String(), Description: p.Description(), TypeHint: p.TypeHint().String(),
		}
	}
	return Snapshot{Kind: KindWidgetType, ID: wt.ID().UUID(), State: state}
}

// SnapshotOfLibraryPin is the snapshot of a pin; its ID is the Library's.
func SnapshotOfLibraryPin(p librarypin.LibraryPin) Snapshot {
	return Snapshot{Kind: KindLibraryPin, ID: p.LibraryID().UUID(), State: LibraryPinState{
		LibraryID: p.LibraryID().UUID(), Release: p.Release().Int(),
	}}
}

// Tag returns the Tag definition the snapshot carries, at the Project
// projectID. It is how a write of a restored snapshot (and undo, ADR 0017)
// rebuilds the entity.
func (s Snapshot) Tag(projectID id.ID[project.Project]) (tag.Tag, error) {
	state, ok := s.State.(TagState)
	if !ok {
		return nil, fmt.Errorf("snapshot of %s carries %T, not a tag", s.ID, s.State)
	}
	name, err := tag.NewTagName(state.Name)
	if err != nil {
		return nil, fmt.Errorf("snapshot of tag %s: %w", s.ID, err)
	}
	tagType, err := tag.NewTagType(state.Type)
	if err != nil {
		return nil, fmt.Errorf("snapshot of tag %s: %w", s.ID, err)
	}
	supplier := tag.NoSupplier()
	if state.Supplier != nil {
		supplier = tag.SuppliedByDevice(id.NewID(id.IDWithUUID[device.Device](*state.Supplier)))
	}
	return tag.ReconstituteTag(id.NewID(id.IDWithUUID[tag.Tag](s.ID)), projectID, name, tagType, supplier)
}

// Device returns the Device the snapshot carries, at the Project projectID.
func (s Snapshot) Device(projectID id.ID[project.Project]) (device.Device, error) {
	state, ok := s.State.(DeviceState)
	if !ok {
		return nil, fmt.Errorf("snapshot of %s carries %T, not a device", s.ID, s.State)
	}
	name, err := device.NewDeviceName(state.Name)
	if err != nil {
		return nil, fmt.Errorf("snapshot of device %s: %w", s.ID, err)
	}
	protocol, err := device.NewDeviceProtocol(state.Protocol)
	if err != nil {
		return nil, fmt.Errorf("snapshot of device %s: %w", s.ID, err)
	}
	config, err := device.NewDeviceConfig(state.Config)
	if err != nil {
		return nil, fmt.Errorf("snapshot of device %s: %w", s.ID, err)
	}
	return device.ReconstituteDevice(id.NewID(id.IDWithUUID[device.Device](s.ID)), projectID, name, state.Comment, protocol, config)
}

// Environment returns the Environment the snapshot carries, at the Project
// projectID.
func (s Snapshot) Environment(projectID id.ID[project.Project]) (device.Environment, error) {
	state, ok := s.State.(EnvironmentState)
	if !ok {
		return nil, fmt.Errorf("snapshot of %s carries %T, not an environment", s.ID, s.State)
	}
	name, err := device.NewEnvironmentName(state.Name)
	if err != nil {
		return nil, fmt.Errorf("snapshot of environment %s: %w", s.ID, err)
	}
	return device.ReconstituteEnvironment(id.NewID(id.IDWithUUID[device.Environment](s.ID)), projectID, name, state.Comment)
}

// EnvironmentDevice returns the pair the snapshot carries, at the Project
// projectID.
func (s Snapshot) EnvironmentDevice(projectID id.ID[project.Project]) (device.EnvironmentDevice, error) {
	state, ok := s.State.(EnvironmentDeviceState)
	if !ok {
		return nil, fmt.Errorf("snapshot of %s carries %T, not an environment device", s.ID, s.State)
	}
	params, err := device.NewEnvironmentDeviceParams(state.Params)
	if err != nil {
		return nil, fmt.Errorf("snapshot of pair %s/%s: %w", state.EnvironmentID, state.DeviceID, err)
	}
	return device.ReconstituteEnvironmentDevice(projectID,
		id.NewID(id.IDWithUUID[device.Environment](state.EnvironmentID)),
		id.NewID(id.IDWithUUID[device.Device](state.DeviceID)),
		params), nil
}

// WidgetType returns the WidgetType the snapshot carries, of the Library
// libraryID. It is how a write of a restored snapshot (and undo, ADR 0017)
// rebuilds the entity.
func (s Snapshot) WidgetType(libraryID id.ID[library.Library]) (library.WidgetType, error) {
	state, ok := s.State.(WidgetTypeState)
	if !ok {
		return nil, fmt.Errorf("snapshot of %s carries %T, not a widget type", s.ID, s.State)
	}
	name, err := library.NewWidgetTypeName(state.Name)
	if err != nil {
		return nil, fmt.Errorf("snapshot of widget type %s: %w", s.ID, err)
	}
	html, err := library.NewHtmlTemplate(state.HtmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("snapshot of widget type %s: %w", s.ID, err)
	}
	script, err := library.NewScript(state.Script)
	if err != nil {
		return nil, fmt.Errorf("snapshot of widget type %s: %w", s.ID, err)
	}
	language, err := library.NewScriptLanguage(state.ScriptLanguage)
	if err != nil {
		return nil, fmt.Errorf("snapshot of widget type %s: %w", s.ID, err)
	}
	size, err := library.NewSize(state.DefaultWidth, state.DefaultHeight)
	if err != nil {
		return nil, fmt.Errorf("snapshot of widget type %s: %w", s.ID, err)
	}
	ports := make([]library.InputPort, len(state.InputPorts))
	for i, p := range state.InputPorts {
		portName, err := library.NewInputPortName(p.Name)
		if err != nil {
			return nil, fmt.Errorf("snapshot of widget type %s: %w", s.ID, err)
		}
		hint, err := library.NewPortTypeHint(p.TypeHint)
		if err != nil {
			return nil, fmt.Errorf("snapshot of widget type %s: %w", s.ID, err)
		}
		ports[i] = library.NewInputPort(portName, p.Description, hint)
	}
	return library.ReconstituteWidgetType(id.NewID(id.IDWithUUID[library.WidgetType](s.ID)),
		libraryID, name, html, script, language, size, ports)
}

// LibraryPin returns the pin the snapshot carries, at the Project projectID.
func (s Snapshot) LibraryPin(projectID id.ID[project.Project]) (librarypin.LibraryPin, error) {
	state, ok := s.State.(LibraryPinState)
	if !ok {
		return nil, fmt.Errorf("snapshot of %s carries %T, not a library pin", s.ID, s.State)
	}
	number, err := library.NewReleaseNumber(state.Release)
	if err != nil {
		return nil, fmt.Errorf("snapshot of pin %s: %w", state.LibraryID, err)
	}
	return librarypin.ReconstituteLibraryPin(projectID,
		id.NewID(id.IDWithUUID[library.Library](state.LibraryID)), number), nil
}
