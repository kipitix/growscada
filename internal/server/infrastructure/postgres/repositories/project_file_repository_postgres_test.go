package repositories_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// sameDocument compares two project documents by their JSON shape: PostgreSQL
// stores a JSON object normalized (key order, spacing), so the bytes of a
// document read back need not be the bytes it was written with.
func sameDocument(t *testing.T, got, want projectv0.ProjectFile) bool {
	t.Helper()
	var gotValue, wantValue any
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal the stored document: %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal the document: %v", err)
	}
	if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
		t.Fatalf("decode the stored document: %v", err)
	}
	if err := json.Unmarshal(wantJSON, &wantValue); err != nil {
		t.Fatalf("decode the document: %v", err)
	}
	return reflect.DeepEqual(gotValue, wantValue)
}

func projectFileOf(projectID uuid.UUID) projectv0.ProjectFile {
	deviceID, environmentID, widgetTypeID := uuid.New(), uuid.New(), uuid.New()
	return projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID},
		Tags: []projectv0.TagDefinition{
			{ID: uuid.New(), Name: "speed", Type: "integer", SupplierDeviceID: &deviceID},
			{ID: uuid.New(), Name: "started", Type: "boolean"},
		},
		Devices: []projectv0.Device{
			{ID: deviceID, Name: "meter", Comment: "the bench meter", Protocol: "simulation", Config: projectv0.JSONObject(`{"host":"meter"}`)},
		},
		Environments: []projectv0.Environment{
			{ID: environmentID, Name: "bench", Comment: "the lab"},
		},
		EnvironmentDevices: []projectv0.EnvironmentDevice{
			{EnvironmentID: environmentID, DeviceID: deviceID, Params: projectv0.JSONObject(`{"address":"10.0.0.1"}`)},
		},
		Scenes: []projectv0.Scene{{
			ID: uuid.New(), Name: "overview", Width: 1920, Height: 1080, BackgroundHTML: "<p>background</p>",
			Widgets: []projectv0.Widget{{
				ID: uuid.New(), Name: "gauge",
				Position: projectv0.Position{X: 10, Y: 20, Z: 1},
				Size:     projectv0.Size{Width: 100, Height: 50},
				Origin:   projectv0.Origin{X: 0.5, Y: 0.5},
				Rotation: projectv0.Rotation{Degrees: 90},
				TypeID:   widgetTypeID,
				Labels:   []string{"speed"},
				PortBindings: []projectv0.PortBinding{{
					PortName: "value", TagID: uuid.New(),
				}},
			}},
		}},
		WidgetTypes: []projectv0.WidgetType{{
			ID: widgetTypeID, LibraryID: uuid.New(), Name: "gauge",
			HTMLTemplate: "<div></div>", Script: "render(inputs)", ScriptLanguage: "javascript",
			DefaultWidth: 100, DefaultHeight: 50,
			InputPorts: []projectv0.InputPort{{Name: "value", Description: "the value", TypeHint: "integer"}},
		}},
		LibraryPins: []projectv0.LibraryPin{{LibraryID: uuid.New(), Release: 3}},
	}
}

func TestProjectFileRepository_SavesAndFindsTheLastProjectFile(t *testing.T) {
	cleanProjects(t)
	project := mustCreateProject(t, repositories.NewProjectRepositoryPostgres(testDB), "Deployed")
	repo := repositories.NewProjectFileRepositoryPostgres(testDB)
	ctx := context.Background()

	if _, ok, err := repo.FindByProject(ctx, project.ID().UUID()); err != nil {
		t.Fatalf("FindByProject: %v", err)
	} else if ok {
		t.Fatal("a project that was never deployed has a project file")
	}

	file := projectFileOf(project.ID().UUID())
	at := time.Date(2026, 11, 3, 10, 30, 0, 0, time.UTC)
	if err := repo.Save(ctx, project.ID().UUID(), appdto.Revision{Number: 1, DeployedAt: at, Content: file}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	found, ok, err := repo.FindByProject(ctx, project.ID().UUID())
	if err != nil {
		t.Fatalf("FindByProject: %v", err)
	}
	if !ok {
		t.Fatal("the project file saved is not found")
	}
	if found.Number != 1 {
		t.Errorf("revision %d, want 1", found.Number)
	}
	if !found.DeployedAt.Equal(at) {
		t.Errorf("deployed at %s, want %s", found.DeployedAt, at)
	}
	if !sameDocument(t, found.Content, file) {
		t.Errorf("the project file read back differs from the one saved:\ngot  %+v\nwant %+v", found.Content, file)
	}

	next := file
	next.Tags = nil
	if err := repo.Save(ctx, project.ID().UUID(), appdto.Revision{Number: 2, DeployedAt: at.Add(time.Hour), Content: next}); err != nil {
		t.Fatalf("Save the next revision: %v", err)
	}
	found, _, err = repo.FindByProject(ctx, project.ID().UUID())
	if err != nil {
		t.Fatalf("FindByProject: %v", err)
	}
	if found.Number != 2 {
		t.Errorf("revision %d after the next deploy, want 2", found.Number)
	}
	if !sameDocument(t, found.Content, next) {
		t.Error("the copy of the project file was not replaced by the next deploy")
	}
}

func TestProjectFileRepository_RefusesTheFileOfAnotherProject(t *testing.T) {
	cleanProjects(t)
	project := mustCreateProject(t, repositories.NewProjectRepositoryPostgres(testDB), "Mismatched")
	repo := repositories.NewProjectFileRepositoryPostgres(testDB)

	file := projectFileOf(uuid.New())
	if err := repo.Save(context.Background(), project.ID().UUID(), appdto.Revision{Number: 1, DeployedAt: time.Now(), Content: file}); err == nil {
		t.Fatal("the project file of another project was stored")
	}
}

func TestProjectFileRepository_AnOlderDeployChangesNothing(t *testing.T) {
	cleanProjects(t)
	project := mustCreateProject(t, repositories.NewProjectRepositoryPostgres(testDB), "Racing")
	repo := repositories.NewProjectFileRepositoryPostgres(testDB)
	ctx := context.Background()
	at := time.Date(2026, 11, 3, 11, 0, 0, 0, time.UTC)

	second := projectFileOf(project.ID().UUID())
	second.Tags = nil
	if err := repo.Save(ctx, project.ID().UUID(), appdto.Revision{Number: 2, DeployedAt: at, Content: second}); err != nil {
		t.Fatalf("Save revision 2: %v", err)
	}
	first := projectFileOf(project.ID().UUID())
	if err := repo.Save(ctx, project.ID().UUID(), appdto.Revision{Number: 1, DeployedAt: at.Add(-time.Hour), Content: first}); err != nil {
		t.Fatalf("Save revision 1 after revision 2: %v", err)
	}
	found, ok, err := repo.FindByProject(ctx, project.ID().UUID())
	if err != nil {
		t.Fatalf("FindByProject: %v", err)
	}
	if !ok || found.Number != 2 {
		t.Fatalf("the copy is revision %d (%v), want 2: a late deploy of an older one is no news", found.Number, ok)
	}
	if !sameDocument(t, found.Content, second) {
		t.Error("the late deploy of revision 1 replaced the copy of revision 2")
	}
}

func TestProjectFileRepository_TheProjectTakesItsProjectFileAlong(t *testing.T) {
	cleanProjects(t)
	project := mustCreateProject(t, repositories.NewProjectRepositoryPostgres(testDB), "Removed")
	repo := repositories.NewProjectFileRepositoryPostgres(testDB)
	ctx := context.Background()

	file := projectFileOf(project.ID().UUID())
	if err := repo.Save(ctx, project.ID().UUID(), appdto.Revision{Number: 1, DeployedAt: time.Now(), Content: file}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, project.ID().UUID()); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if _, ok, err := repo.FindByProject(ctx, project.ID().UUID()); err != nil {
		t.Fatalf("FindByProject: %v", err)
	} else if ok {
		t.Error("the project file of a deleted project survives it")
	}
}
