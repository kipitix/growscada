package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// ProjectFileBuilder assembles the ProjectFile of a Project's Draft (task 35,
// Q2, Q6): the whole Draft — its Tags with their supplier, its Devices, its
// Environments with the pairs of those Devices, its Scenes with their Widgets
// — the copies of the WidgetTypes its Widgets use, taken from the Releases its
// Library Pins hold, and the pins themselves as provenance.
//
// The document is canonical: every list is sorted by ID and every carried
// JSON object has its keys sorted, so the same Draft is always the same bytes.
// That is what tells an unchanged Draft from a changed one and what a Deploy
// stores.
type ProjectFileBuilder interface {
	// Build returns the ProjectFile of the Draft of the live Project
	// projectID. It is an error wrapping project.ErrProjectNotFound if the
	// Project is not live, and a build error if a Widget refers to a
	// WidgetType none of the pinned Releases holds or a Tag is supplied by a
	// Device that is not of the Project: the Draft is inconsistent, and a
	// Deploy must not paper over it.
	Build(ctx context.Context, projectID uuid.UUID) (projectv0.ProjectFile, error)
}

type projectFileBuilder struct {
	content ProjectContent
}

var _ ProjectFileBuilder = (*projectFileBuilder)(nil)

// NewProjectFileBuilder returns the builder of the ProjectFiles of the live
// Projects the ProjectContent serves.
func NewProjectFileBuilder(aContent ProjectContent) ProjectFileBuilder {
	return &projectFileBuilder{content: aContent}
}

func (b projectFileBuilder) Build(ctx context.Context, rawProjectID uuid.UUID) (projectv0.ProjectFile, error) {
	projectID, scope, err := b.content.open(ctx, rawProjectID)
	if err != nil {
		return projectv0.ProjectFile{}, err
	}

	tags, err := scope.Tags().FindAll(ctx)
	if err != nil {
		return projectv0.ProjectFile{}, fmt.Errorf("cannot read the tags of project %s: %w", projectID, err)
	}
	devices, err := scope.Devices().FindAll(ctx)
	if err != nil {
		return projectv0.ProjectFile{}, fmt.Errorf("cannot read the devices of project %s: %w", projectID, err)
	}
	environments, err := scope.Environments().FindAll(ctx)
	if err != nil {
		return projectv0.ProjectFile{}, fmt.Errorf("cannot read the environments of project %s: %w", projectID, err)
	}
	pairs, err := scope.EnvironmentDevices().FindAll(ctx)
	if err != nil {
		return projectv0.ProjectFile{}, fmt.Errorf("cannot read the environment devices of project %s: %w", projectID, err)
	}
	scenes, err := scope.Scenes().FindAll(ctx)
	if err != nil {
		return projectv0.ProjectFile{}, fmt.Errorf("cannot read the scenes of project %s: %w", projectID, err)
	}
	pins, err := scope.LibraryPins().FindAll(ctx)
	if err != nil {
		return projectv0.ProjectFile{}, fmt.Errorf("cannot read the library pins of project %s: %w", projectID, err)
	}

	return projectFileOfContent(projectID, draft.Content{
		Tags:               tags,
		Devices:            devices,
		Environments:       environments,
		EnvironmentDevices: pairs,
		Scenes:             scenes,
		LibraryPins:        pins,
	}, func(typeID id.ID[library.WidgetType]) (projectv0.WidgetType, error) {
		pinned, err := scope.LibraryPins().FindPinnedWidgetType(ctx, typeID)
		if err != nil {
			if errors.Is(err, library.ErrWidgetTypeNotFound) {
				return projectv0.WidgetType{}, err
			}
			return projectv0.WidgetType{}, fmt.Errorf("cannot read widget type %s of project %s: %w", typeID, projectID, err)
		}
		return newProjectFileWidgetType(pinned), nil
	})
}

// widgetTypeResolver returns the copy of a WidgetType a ProjectFile carries:
// the pinned Release's copy for a Draft, the document's own copy for a file
// that is validated (task 39, ADR 0018).
type widgetTypeResolver func(typeID id.ID[library.WidgetType]) (projectv0.WidgetType, error)

// projectFileOfContent assembles the canonical ProjectFile of content, a
// Draft's content or the content a project file would make: the same build
// the Deploy sends and the validation compares a file with. A dangling
// reference — a Tag supplied by a Device outside content, a PortBinding to a
// Tag outside it, a Widget of a WidgetType the resolver does not hold — is an
// error: the configuration is not deployable.
func projectFileOfContent(projectID id.ID[project.Project], content draft.Content, resolve widgetTypeResolver) (projectv0.ProjectFile, error) {
	file := projectv0.ProjectFile{
		Header:             projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID.UUID()},
		Tags:               make([]projectv0.TagDefinition, 0, len(content.Tags)),
		Devices:            make([]projectv0.Device, 0, len(content.Devices)),
		Environments:       make([]projectv0.Environment, 0, len(content.Environments)),
		EnvironmentDevices: make([]projectv0.EnvironmentDevice, 0, len(content.EnvironmentDevices)),
		Scenes:             make([]projectv0.Scene, 0, len(content.Scenes)),
		WidgetTypes:        []projectv0.WidgetType{},
		LibraryPins:        make([]projectv0.LibraryPin, 0, len(content.LibraryPins)),
	}

	deviceIDs := make(map[uuid.UUID]struct{}, len(content.Devices))
	for _, d := range content.Devices {
		deviceIDs[d.ID().UUID()] = struct{}{}
		file.Devices = append(file.Devices, newProjectFileDevice(d))
	}
	tagIDs := make(map[uuid.UUID]struct{}, len(content.Tags))
	for _, t := range content.Tags {
		tagIDs[t.ID().UUID()] = struct{}{}
		if supplier := t.Supplier(); supplier.HasDevice() {
			if _, ok := deviceIDs[supplier.DeviceID().UUID()]; !ok {
				return projectv0.ProjectFile{}, fmt.Errorf(
					"cannot build the project file of %s: tag %s is supplied by device %s, which is not of the project",
					projectID, t.ID(), supplier.DeviceID())
			}
		}
		file.Tags = append(file.Tags, newProjectFileTag(t))
	}
	for _, e := range content.Environments {
		file.Environments = append(file.Environments, projectv0.Environment{
			ID: e.ID().UUID(), Name: e.Name().String(), Comment: e.Comment(),
		})
	}
	for _, pair := range content.EnvironmentDevices {
		file.EnvironmentDevices = append(file.EnvironmentDevices, projectv0.EnvironmentDevice{
			EnvironmentID: pair.EnvironmentID().UUID(),
			DeviceID:      pair.DeviceID().UUID(),
			Params:        projectv0.JSONObject(pair.Params().Bytes()),
		})
	}
	for _, pin := range content.LibraryPins {
		file.LibraryPins = append(file.LibraryPins, projectv0.LibraryPin{
			LibraryID: pin.LibraryID().UUID(), Release: pin.Release().Int(),
		})
	}

	// usedTypes are the WidgetTypes the Widgets are of; a copy of one is
	// carried once.
	usedTypes := make(map[uuid.UUID]struct{})
	for _, sc := range content.Scenes {
		fileScene := projectv0.Scene{
			ID: sc.ID().UUID(), Name: sc.Name().String(),
			Width: sc.Size().Width(), Height: sc.Size().Height(),
			BackgroundHTML: sc.BackgroundHTML().Content(),
			Widgets:        make([]projectv0.Widget, 0, len(sc.Widgets())),
		}
		for _, w := range sc.Widgets() {
			typeID := w.TypeID().UUID()
			if _, ok := usedTypes[typeID]; !ok {
				copied, err := resolve(w.TypeID())
				if err != nil {
					if errors.Is(err, library.ErrWidgetTypeNotFound) {
						return projectv0.ProjectFile{}, fmt.Errorf(
							"cannot build the project file of %s: widget %s of scene %s is of widget type %s, which no pinned release holds",
							projectID, w.ID(), sc.ID(), w.TypeID())
					}
					return projectv0.ProjectFile{}, err
				}
				file.WidgetTypes = append(file.WidgetTypes, copied)
				usedTypes[typeID] = struct{}{}
			}
			for _, binding := range w.PortBindings() {
				if _, ok := tagIDs[binding.TagID().UUID()]; !ok {
					return projectv0.ProjectFile{}, fmt.Errorf(
						"cannot build the project file of %s: widget %s of scene %s binds port %s to tag %s, which is not of the project",
						projectID, w.ID(), sc.ID(), binding.PortName(), binding.TagID())
				}
			}
			fileScene.Widgets = append(fileScene.Widgets, newProjectFileWidget(w))
		}
		file.Scenes = append(file.Scenes, fileScene)
	}

	return canonicalProjectFile(file), nil
}

func newProjectFileTag(t tag.Tag) projectv0.TagDefinition {
	def := projectv0.TagDefinition{ID: t.ID().UUID(), Name: t.Name().String(), Type: t.Type().String()}
	if supplier := t.Supplier(); supplier.HasDevice() {
		deviceID := supplier.DeviceID().UUID()
		def.SupplierDeviceID = &deviceID
	}
	return def
}

func newProjectFileDevice(d device.Device) projectv0.Device {
	return projectv0.Device{
		ID: d.ID().UUID(), Name: d.Name().String(), Comment: d.Comment(),
		Protocol: d.Protocol().String(), Config: projectv0.JSONObject(d.Config().Bytes()),
	}
}

func newProjectFileWidget(w scene.Widget) projectv0.Widget {
	bindings := w.PortBindings()
	portBindings := make([]projectv0.PortBinding, len(bindings))
	for i, b := range bindings {
		portBindings[i] = projectv0.PortBinding{PortName: b.PortName().String(), TagID: b.TagID().UUID()}
	}
	return projectv0.Widget{
		ID:   w.ID().UUID(),
		Name: w.Name().String(),
		Position: projectv0.Position{
			X: w.Position().X(), Y: w.Position().Y(), Z: w.Position().Z(),
		},
		Size:         projectv0.Size{Width: w.Size().Width(), Height: w.Size().Height()},
		Origin:       projectv0.Origin{X: w.Origin().X(), Y: w.Origin().Y()},
		Rotation:     projectv0.Rotation{Degrees: w.Rotation().Degrees()},
		TypeID:       w.TypeID().UUID(),
		Labels:       w.Labels(),
		PortBindings: portBindings,
	}
}

func newProjectFileWidgetType(wt library.ReleasedWidgetType) projectv0.WidgetType {
	ports := wt.InputPorts()
	inputPorts := make([]projectv0.InputPort, len(ports))
	for i, p := range ports {
		inputPorts[i] = projectv0.InputPort{
			Name: p.Name().String(), Description: p.Description(), TypeHint: p.TypeHint().String(),
		}
	}
	return projectv0.WidgetType{
		ID:             wt.ID().UUID(),
		LibraryID:      wt.LibraryID().UUID(),
		Name:           wt.Name().String(),
		HTMLTemplate:   wt.HtmlTemplate().String(),
		Script:         wt.Script().String(),
		ScriptLanguage: wt.ScriptLanguage().String(),
		DefaultWidth:   wt.DefaultSize().Width(),
		DefaultHeight:  wt.DefaultSize().Height(),
		InputPorts:     inputPorts,
	}
}

// canonicalProjectFile returns file with every list sorted by ID — the
// EnvironmentDevices by their pair, the Widgets of a Scene by their ID — and
// every carried JSON object canonical (canonicalJSONObject). Marshaling the
// result is then the same bytes for every Draft of the same configuration.
func canonicalProjectFile(file projectv0.ProjectFile) projectv0.ProjectFile {
	// A list the document carries as null and one it carries as [] are the
	// same empty list: the builder always emits [].
	file.Tags = cloneOrEmpty(file.Tags)
	file.Devices = cloneOrEmpty(file.Devices)
	file.Environments = cloneOrEmpty(file.Environments)
	file.EnvironmentDevices = cloneOrEmpty(file.EnvironmentDevices)
	file.Scenes = cloneOrEmpty(file.Scenes)
	file.WidgetTypes = cloneOrEmpty(file.WidgetTypes)
	file.LibraryPins = cloneOrEmpty(file.LibraryPins)

	slices.SortFunc(file.Tags, func(a, b projectv0.TagDefinition) int { return compareIDs(a.ID, b.ID) })
	slices.SortFunc(file.Devices, func(a, b projectv0.Device) int { return compareIDs(a.ID, b.ID) })
	slices.SortFunc(file.Environments, func(a, b projectv0.Environment) int { return compareIDs(a.ID, b.ID) })
	slices.SortFunc(file.EnvironmentDevices, func(a, b projectv0.EnvironmentDevice) int {
		if c := compareIDs(a.EnvironmentID, b.EnvironmentID); c != 0 {
			return c
		}
		return compareIDs(a.DeviceID, b.DeviceID)
	})
	slices.SortFunc(file.Scenes, func(a, b projectv0.Scene) int { return compareIDs(a.ID, b.ID) })
	slices.SortFunc(file.WidgetTypes, func(a, b projectv0.WidgetType) int { return compareIDs(a.ID, b.ID) })
	slices.SortFunc(file.LibraryPins, func(a, b projectv0.LibraryPin) int { return compareIDs(a.LibraryID, b.LibraryID) })

	for i := range file.Devices {
		file.Devices[i].Config = canonicalJSONObject(file.Devices[i].Config)
	}
	for i := range file.EnvironmentDevices {
		file.EnvironmentDevices[i].Params = canonicalJSONObject(file.EnvironmentDevices[i].Params)
	}
	for i := range file.Scenes {
		file.Scenes[i].Widgets = cloneOrEmpty(file.Scenes[i].Widgets)
		slices.SortFunc(file.Scenes[i].Widgets, func(a, b projectv0.Widget) int { return compareIDs(a.ID, b.ID) })
		for j := range file.Scenes[i].Widgets {
			file.Scenes[i].Widgets[j].Labels = cloneOrEmpty(file.Scenes[i].Widgets[j].Labels)
			file.Scenes[i].Widgets[j].PortBindings = cloneOrEmpty(file.Scenes[i].Widgets[j].PortBindings)
		}
	}
	for i := range file.WidgetTypes {
		file.WidgetTypes[i].InputPorts = cloneOrEmpty(file.WidgetTypes[i].InputPorts)
	}
	return file
}

// cloneOrEmpty returns a copy of list, the empty list for a nil one.
func cloneOrEmpty[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return slices.Clone(list)
}

// compareIDs orders two UUIDs.
func compareIDs(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) }

// canonicalJSONObject returns obj with its keys sorted, recursively, and no
// insignificant space: two objects that differ only in the order of their
// keys are the same object. Numbers keep their literal form, so a value read
// back from storage compares equal to the one a Deploy wrote.
func canonicalJSONObject(obj projectv0.JSONObject) projectv0.JSONObject {
	if len(obj) == 0 {
		return projectv0.JSONObject("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(obj))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		// A value stored or built as JSONObject is an object already
		// (UnmarshalJSON and the domain constructors check it); one that is
		// not is carried as it is rather than failing the whole document.
		return projectv0.JSONObject(slices.Clone(obj))
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return projectv0.JSONObject(slices.Clone(obj))
	}
	return projectv0.JSONObject(canonical)
}

// sameProjectFile reports whether two documents are the same configuration:
// their canonical forms are the same bytes.
func sameProjectFile(a, b projectv0.ProjectFile) (bool, error) {
	left, err := json.Marshal(canonicalProjectFile(a))
	if err != nil {
		return false, fmt.Errorf("cannot encode the project file of %s: %w", a.ProjectID, err)
	}
	right, err := json.Marshal(canonicalProjectFile(b))
	if err != nil {
		return false, fmt.Errorf("cannot encode the project file of %s: %w", b.ProjectID, err)
	}
	return bytes.Equal(left, right), nil
}
