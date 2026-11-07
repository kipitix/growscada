// Package player is the UI's operational model: the Deployment a Project runs,
// the states of its Tags and the position of its Journal, fed by a starting
// state and the Journal entries that follow. A Player does not know whether
// its entries are live or replayed (ADR 0020): Operation instantiates it over
// the live stream, History (task 45) will instantiate it over a playback.
//
// The package is the UI layer's alone: it maps the Deployment's ProjectFile to
// the shapes livescene renders and imports no server domain.
package player

import (
	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/livescene"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// TagState is the state of one Tag as the Player knows it. It is kept for
// every Tag the Player ever heard of, including one no longer in the running
// configuration: a state is keyed by the Tag's identity, so a later Deployment
// that names the Tag again finds it (ADR 0020). A state outside the
// configuration is never rendered.
type TagState struct {
	Value     string
	Quality   string
	Seq       int64
	WrittenAt *string
}

// Result is what applying one Journal entry did.
type Result struct {
	// Changed reports whether the rendered picture changed: a Tag state the
	// Player applies.
	Changed bool
	// Deployment is the Deployment whose document must be read
	// (GET …/deployments/{n}) and applied, 0 when no document is needed. The
	// entry carries only the number: the document is immutable and lives in
	// the Runtime node's store (ADR 0014).
	Deployment int
}

// Model is the operational picture of one Project: the Deployment it runs, the
// states of its Tags by ID, its Journal position and whether its stream is
// connected. The zero value is a Project without a Deployment, no states and
// position 0. Everything here is owned by one Player and lives on the UI
// goroutine.
type Model struct {
	position   int64
	deployment *apiv0.DeploymentResponse
	states     map[uuid.UUID]TagState
	connected  bool

	// The render forms of deployment.Content, rebuilt when the Deployment changes.
	scenes      []apiv0.SceneResponse
	widgetTypes map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse
	widgets     map[uuid.UUID][]apiv0.WidgetResponse
}

// Position returns the Journal position the Player has consumed: the position
// a stream starts after.
func (m *Model) Position() int64 { return m.position }

// Deployed reports whether the Project runs a Deployment.
func (m *Model) Deployed() bool { return m.deployment != nil }

// Connected reports whether the Player's stream is up.
func (m *Model) Connected() bool { return m.connected }

// SetConnected records the state of the stream and reports whether it changed.
func (m *Model) SetConnected(connected bool) bool {
	if m.connected == connected {
		return false
	}
	m.connected = connected
	return true
}

// Deployment returns the Deployment the Project runs, nil until its first Deploy.
// The document is for reading only.
func (m *Model) Deployment() *apiv0.DeploymentResponse { return m.deployment }

// TagState returns the known state of the Tag id, false if none was heard of.
func (m *Model) TagState(id uuid.UUID) (TagState, bool) {
	state, ok := m.states[id]
	return state, ok
}

// States returns the states of every Tag the Player heard of, by ID, for
// reading only. It holds Tags outside the running configuration too.
func (m *Model) States() map[uuid.UUID]TagState { return m.states }

// MergeSnapshot applies a starting state (GET …/operation, or a checkpoint of
// a playback): the position moves forward, a Deployment with a greater number
// wins, and for every Tag the state with the greater Seq wins. Entries that
// arrived while the starting state was being read are therefore never rolled
// back by it (ADR 0020).
func (m *Model) MergeSnapshot(snapshot apiv0.OperationStateResponse) {
	if snapshot.Position > m.position {
		m.position = snapshot.Position
	}
	if snapshot.Deployment != nil {
		m.SetDeployment(*snapshot.Deployment)
	}
	for key, state := range snapshot.TagStates {
		id, err := uuid.Parse(key)
		if err != nil || id == uuid.Nil {
			continue
		}
		if current, ok := m.states[id]; ok && current.Seq >= state.Seq {
			continue
		}
		if m.states == nil {
			m.states = make(map[uuid.UUID]TagState)
		}
		m.states[id] = TagState{
			Value:     state.Value,
			Quality:   state.Quality,
			Seq:       state.Seq,
			WrittenAt: state.WrittenAt,
		}
	}
}

// SetDeployment swaps the configuration to deployment and reports whether it did.
// A repeated or older Deployment is ignored; the Tag states are kept by ID, so
// a new Tag shows its type's zero value with Quality bad until it is written
// and a removed Tag is no longer rendered.
func (m *Model) SetDeployment(deployment apiv0.DeploymentResponse) bool {
	if m.deployment != nil && deployment.Number <= m.deployment.Number {
		return false
	}
	document := deployment
	m.deployment = &document

	content := document.Content
	m.scenes = make([]apiv0.SceneResponse, len(content.Scenes))
	m.widgets = make(map[uuid.UUID][]apiv0.WidgetResponse, len(content.Scenes))
	for i, scene := range content.Scenes {
		m.scenes[i] = sceneOf(scene)
		widgets := make([]apiv0.WidgetResponse, len(scene.Widgets))
		for j, widget := range scene.Widgets {
			widgets[j] = widgetOf(widget, scene.ID)
		}
		m.widgets[scene.ID] = widgets
	}
	m.widgetTypes = make(map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse, len(content.WidgetTypes))
	for _, widgetType := range content.WidgetTypes {
		m.widgetTypes[widgetType.ID] = widgetTypeOf(widgetType)
	}
	return true
}

// Apply applies one Journal entry and reports what it changed and what the
// Player must read next. The position advances to every entry consumed, even
// one this reader cannot apply (an unknown type, a foreign MAJOR): the
// position is a cursor, and a reconnect must not replay entries forever. The
// order the entries arrive in is kept by the caller.
func (m *Model) Apply(entry recordv0.Entry) Result {
	if entry.Seq > m.position {
		m.position = entry.Seq
	}
	if entry.SchemaVersion.Major != recordv0.SchemaVersion().Major {
		return Result{}
	}
	switch entry.Type {
	case recordv0.EntryTypeTagState:
		return m.applyTagState(entry)
	case recordv0.EntryTypeDeployment:
		if entry.Deployment == nil {
			return Result{}
		}
		if m.deployment != nil && entry.Deployment.Number <= m.deployment.Number {
			return Result{}
		}
		return Result{Deployment: entry.Deployment.Number}
	}
	return Result{}
}

// applyTagState records the state a tag_state entry carries, unless the
// known state has the same or a greater Seq: entries and starting states may
// arrive out of order, and for one Tag the greater position wins.
func (m *Model) applyTagState(entry recordv0.Entry) Result {
	if entry.TagState == nil || entry.TagState.TagID == uuid.Nil {
		return Result{}
	}
	id := entry.TagState.TagID
	if current, ok := m.states[id]; ok && current.Seq >= entry.Seq {
		return Result{}
	}
	state := TagState{
		Value:   entry.TagState.Value,
		Quality: entry.TagState.Quality,
		Seq:     entry.Seq,
	}
	if entry.Timestamp != "" {
		written := entry.Timestamp
		state.WrittenAt = &written
	}
	if m.states == nil {
		m.states = make(map[uuid.UUID]TagState)
	}
	m.states[id] = state
	return Result{Changed: true}
}

// View returns what Operation renders: the Deployment's Scenes and WidgetTypes,
// the Tags of its configuration with their states and the state of the
// connection. A View shares the Model's maps for reading only.
type View struct {
	Deployment  *apiv0.DeploymentResponse
	Scenes      []apiv0.SceneResponse
	WidgetTypes map[uuid.UUID]apiv0.ReleasedWidgetTypeResponse
	Tags        map[uuid.UUID]apiv0.TagResponse
	Connected   bool

	widgets map[uuid.UUID][]apiv0.WidgetResponse
}

// Deployed reports whether there is a Deployment to render.
func (v View) Deployed() bool { return v.Deployment != nil }

// Widgets returns the Widgets of the Scene sceneID of the Deployment; nil when
// the Deployment has no such Scene.
func (v View) Widgets(sceneID uuid.UUID) []apiv0.WidgetResponse { return v.widgets[sceneID] }

// View returns the current picture. The Tags are those of the running
// configuration alone; a Tag without a state shows its TagType's zero value
// with Quality bad and Seq 0, as a Tag never written does.
func (m *Model) View() View {
	return View{
		Deployment:  m.deployment,
		Scenes:      m.scenes,
		WidgetTypes: m.widgetTypes,
		Tags:        m.tags(),
		Connected:   m.connected,
		widgets:     m.widgets,
	}
}

// tags are the Tags of the running configuration with their states, by ID.
func (m *Model) tags() map[uuid.UUID]apiv0.TagResponse {
	empty := map[uuid.UUID]apiv0.TagResponse{}
	if m.deployment == nil {
		return empty
	}
	definitions := m.deployment.Content.Tags
	tags := make(map[uuid.UUID]apiv0.TagResponse, len(definitions))
	for _, definition := range definitions {
		state, ok := m.states[definition.ID]
		if !ok {
			state = TagState{
				Value:   zeroValueOf(definition.Type),
				Quality: string(livescene.QualityBad),
			}
		}
		tags[definition.ID] = apiv0.TagResponse{
			ID:        definition.ID,
			Name:      definition.Name,
			Type:      definition.Type,
			Value:     state.Value,
			Quality:   state.Quality,
			Seq:       state.Seq,
			WrittenAt: state.WrittenAt,
		}
	}
	return tags
}

// sceneOf is the Deployment's Scene in the shape the view renders. The Deployment
// has no aggregate versions: the Version is not read.
func sceneOf(scene projectv0.Scene) apiv0.SceneResponse {
	return apiv0.SceneResponse{
		ID:             scene.ID,
		Name:           scene.Name,
		Width:          scene.Width,
		Height:         scene.Height,
		BackgroundHTML: scene.BackgroundHTML,
	}
}

// widgetOf is the Deployment's Widget in the shape the view renders. The
// Deployment has no matrix: it is derived from the geometry (uiutil.MatrixCSS
// mirrors the domain formula).
func widgetOf(widget projectv0.Widget, sceneID uuid.UUID) apiv0.WidgetResponse {
	bindings := make([]apiv0.PortBinding, len(widget.PortBindings))
	for i, binding := range widget.PortBindings {
		bindings[i] = apiv0.PortBinding{PortName: binding.PortName, TagID: binding.TagID}
	}
	return apiv0.WidgetResponse{
		ID:           widget.ID,
		Name:         widget.Name,
		Position:     apiv0.PositionResponse{X: widget.Position.X, Y: widget.Position.Y, Z: widget.Position.Z},
		Size:         apiv0.SizeResponse{Width: widget.Size.Width, Height: widget.Size.Height},
		Origin:       apiv0.OriginResponse{X: widget.Origin.X, Y: widget.Origin.Y},
		Rotation:     apiv0.RotationResponse{Degrees: widget.Rotation.Degrees},
		TypeID:       widget.TypeID,
		SceneID:      sceneID,
		Labels:       widget.Labels,
		PortBindings: bindings,
		TransformMatrix: apiv0.TransformMatrixResponse{
			CSS: uiutil.MatrixCSS(
				widget.Position.X, widget.Position.Y,
				widget.Origin.X, widget.Origin.Y,
				widget.Rotation.Degrees,
				widget.Size.Width, widget.Size.Height,
			),
		},
	}
}

// widgetTypeOf is the Deployment's copy of a WidgetType in the shape the view
// renders.
func widgetTypeOf(widgetType projectv0.WidgetType) apiv0.ReleasedWidgetTypeResponse {
	ports := make([]apiv0.InputPort, len(widgetType.InputPorts))
	for i, port := range widgetType.InputPorts {
		ports[i] = apiv0.InputPort{Name: port.Name, Description: port.Description, TypeHint: port.TypeHint}
	}
	return apiv0.ReleasedWidgetTypeResponse{
		ID:             widgetType.ID,
		LibraryID:      widgetType.LibraryID,
		Name:           widgetType.Name,
		HtmlTemplate:   widgetType.HTMLTemplate,
		Script:         widgetType.Script,
		ScriptLanguage: widgetType.ScriptLanguage,
		DefaultWidth:   widgetType.DefaultWidth,
		DefaultHeight:  widgetType.DefaultHeight,
		InputPorts:     ports,
	}
}

// zeroValueOf is the TagType's zero value as a Tag's value is spelled: a Tag
// of the Deployment the Runtime node holds no state for still renders its type.
func zeroValueOf(tagType string) string {
	switch tagType {
	case "boolean":
		return "false"
	case "integer":
		return "0"
	default:
		return ""
	}
}
