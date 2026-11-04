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
	"github.com/kipitix/growscada/internal/server/domain/library"
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

	file := projectv0.ProjectFile{
		Header:             projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID.UUID()},
		Tags:               make([]projectv0.TagDefinition, 0, len(tags)),
		Devices:            make([]projectv0.Device, 0, len(devices)),
		Environments:       make([]projectv0.Environment, 0, len(environments)),
		EnvironmentDevices: make([]projectv0.EnvironmentDevice, 0, len(pairs)),
		Scenes:             make([]projectv0.Scene, 0, len(scenes)),
		WidgetTypes:        []projectv0.WidgetType{},
		LibraryPins:        make([]projectv0.LibraryPin, 0, len(pins)),
	}

	deviceIDs := make(map[uuid.UUID]struct{}, len(devices))
	for _, d := range devices {
		deviceIDs[d.ID().UUID()] = struct{}{}
		file.Devices = append(file.Devices, newProjectFileDevice(d))
	}
	tagIDs := make(map[uuid.UUID]struct{}, len(tags))
	for _, t := range tags {
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
	for _, e := range environments {
		file.Environments = append(file.Environments, projectv0.Environment{
			ID: e.ID().UUID(), Name: e.Name().String(), Comment: e.Comment(),
		})
	}
	for _, pair := range pairs {
		file.EnvironmentDevices = append(file.EnvironmentDevices, projectv0.EnvironmentDevice{
			EnvironmentID: pair.EnvironmentID().UUID(),
			DeviceID:      pair.DeviceID().UUID(),
			Params:        projectv0.JSONObject(pair.Params().Bytes()),
		})
	}
	for _, pin := range pins {
		file.LibraryPins = append(file.LibraryPins, projectv0.LibraryPin{
			LibraryID: pin.LibraryID().UUID(), Release: pin.Release().Int(),
		})
	}

	// usedTypes are the WidgetTypes the Widgets of the Draft are of, taken
	// from the Releases the Project pins; a copy of one is carried once.
	usedTypes := make(map[uuid.UUID]struct{})
	for _, sc := range scenes {
		fileScene := projectv0.Scene{
			ID: sc.ID().UUID(), Name: sc.Name().String(),
			Width: sc.Size().Width(), Height: sc.Size().Height(),
			BackgroundHTML: sc.BackgroundHTML().Content(),
			Widgets:        make([]projectv0.Widget, 0, len(sc.Widgets())),
		}
		for _, w := range sc.Widgets() {
			typeID := w.TypeID().UUID()
			if _, ok := usedTypes[typeID]; !ok {
				pinned, err := scope.LibraryPins().FindPinnedWidgetType(ctx, w.TypeID())
				if err != nil {
					if errors.Is(err, library.ErrWidgetTypeNotFound) {
						return projectv0.ProjectFile{}, fmt.Errorf(
							"cannot build the project file of %s: widget %s of scene %s is of widget type %s, which no pinned release holds",
							projectID, w.ID(), sc.ID(), w.TypeID())
					}
					return projectv0.ProjectFile{}, fmt.Errorf("cannot read widget type %s of project %s: %w", typeID, projectID, err)
				}
				file.WidgetTypes = append(file.WidgetTypes, newProjectFileWidgetType(pinned))
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
	file.Tags = slices.Clone(file.Tags)
	file.Devices = slices.Clone(file.Devices)
	file.Environments = slices.Clone(file.Environments)
	file.EnvironmentDevices = slices.Clone(file.EnvironmentDevices)
	file.Scenes = slices.Clone(file.Scenes)
	file.WidgetTypes = slices.Clone(file.WidgetTypes)
	file.LibraryPins = slices.Clone(file.LibraryPins)

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
		file.Scenes[i].Widgets = slices.Clone(file.Scenes[i].Widgets)
		slices.SortFunc(file.Scenes[i].Widgets, func(a, b projectv0.Widget) int { return compareIDs(a.ID, b.ID) })
	}
	return file
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
