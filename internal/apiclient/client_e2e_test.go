package apiclient_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/servertest"
)

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

func startClient(t *testing.T) *apiclient.Client {
	t.Helper()
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	return apiclient.New(srv.URL, srv.Client())
}

func TestTags_RoundTrip(t *testing.T) {
	client := startClient(t)
	ctx := context.Background()

	created, err := client.CreateTag(ctx, apiv0.CreateTagRequest{Name: "pump.speed", Type: "integer", Value: "1", Quality: "good"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	got, err := client.GetTag(ctx, created.ID)
	if err != nil || got.Name != "pump.speed" || got.Value != "1" {
		t.Fatalf("GetTag: %+v, %v", got, err)
	}
	version, err := client.SetTagValue(ctx, created.ID, "2", "uncertain", got.Version)
	if err != nil || version <= got.Version {
		t.Fatalf("SetTagValue: %d, %v", version, err)
	}
	list, err := client.ListTags(ctx)
	if err != nil || len(list) != 1 || list[0].Value != "2" || list[0].Quality != "uncertain" {
		t.Fatalf("ListTags: %+v, %v", list, err)
	}
	deleted, err := client.DeleteTag(ctx, created.ID)
	if err != nil || deleted.ID != created.ID {
		t.Fatalf("DeleteTag: %+v, %v", deleted, err)
	}
	if _, err := client.GetTag(ctx, created.ID); !errors.Is(err, apiclient.ErrNotFound) {
		t.Errorf("GetTag after delete: %v, want ErrNotFound", err)
	}
}

func TestWidgetTypes_RoundTrip(t *testing.T) {
	client := startClient(t)
	ctx := context.Background()

	created, err := client.CreateWidgetType(ctx, apiv0.CreateWidgetTypeRequest{
		Name: "Gauge", HtmlTemplate: "<div></div>", Script: "", ScriptLanguage: "javascript",
		DefaultWidth: 100, DefaultHeight: 50,
		InputPorts: []apiv0.InputPort{{Name: "value", TypeHint: "integer"}},
	})
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	got, err := client.GetWidgetType(ctx, created.ID)
	if err != nil || got.Name != "Gauge" || len(got.InputPorts) != 1 {
		t.Fatalf("GetWidgetType: %+v, %v", got, err)
	}
	update := apiv0.UpdateWidgetTypeRequest{
		Name: "Big gauge", HtmlTemplate: got.HtmlTemplate, ScriptLanguage: got.ScriptLanguage,
		DefaultWidth: 200, DefaultHeight: 100, InputPorts: got.InputPorts, Version: got.Version,
	}
	updated, err := client.UpdateWidgetType(ctx, created.ID, update)
	if err != nil || updated.Version <= got.Version {
		t.Fatalf("UpdateWidgetType: %+v, %v", updated, err)
	}
	if _, err := client.UpdateWidgetType(ctx, created.ID, update); !errors.Is(err, apiclient.ErrConflict) {
		t.Errorf("UpdateWidgetType with a stale version: %v, want ErrConflict", err)
	}
	list, err := client.ListWidgetTypes(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "Big gauge" {
		t.Fatalf("ListWidgetTypes: %+v, %v", list, err)
	}
	if deleted, err := client.DeleteWidgetType(ctx, created.ID); err != nil || deleted.ID != created.ID {
		t.Fatalf("DeleteWidgetType: %+v, %v", deleted, err)
	}
	if _, err := client.GetWidgetType(ctx, created.ID); !errors.Is(err, apiclient.ErrNotFound) {
		t.Errorf("GetWidgetType after delete: %v, want ErrNotFound", err)
	}
}

func TestScenesAndWidgets_RoundTrip(t *testing.T) {
	client := startClient(t)
	ctx := context.Background()

	scene, err := client.CreateScene(ctx, apiv0.CreateSceneRequest{Name: "Main", Width: 800, Height: 600})
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	sc, err := client.GetScene(ctx, scene.ID)
	if err != nil || sc.Name != "Main" {
		t.Fatalf("GetScene: %+v, %v", sc, err)
	}
	updatedScene, err := client.UpdateScene(ctx, scene.ID, apiv0.UpdateSceneRequest{Name: "Overview", Width: 800, Height: 600, Version: sc.Version})
	if err != nil || updatedScene.Version <= sc.Version {
		t.Fatalf("UpdateScene: %+v, %v", updatedScene, err)
	}

	wt, err := client.CreateWidgetType(ctx, apiv0.CreateWidgetTypeRequest{Name: "Lamp", ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 10, InputPorts: []apiv0.InputPort{}})
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	widgetReq := apiv0.CreateWidgetRequest{
		Name: "lamp1", Size: apiv0.SizeRequest{Width: 10, Height: 10}, Origin: apiv0.OriginRequest{X: 0.5, Y: 0.5},
		TypeID: wt.ID, SceneVersion: updatedScene.Version, Labels: []string{}, PortBindings: []apiv0.PortBinding{},
	}
	widget, err := client.CreateWidget(ctx, scene.ID, widgetReq)
	if err != nil || widget.SceneVersion <= updatedScene.Version {
		t.Fatalf("CreateWidget: %+v, %v", widget, err)
	}
	if _, err := client.CreateWidget(ctx, scene.ID, widgetReq); !errors.Is(err, apiclient.ErrConflict) {
		t.Errorf("CreateWidget with a stale Scene version: %v, want ErrConflict", err)
	}
	w, err := client.GetWidget(ctx, scene.ID, widget.ID)
	if err != nil || w.Name != "lamp1" || w.SceneVersion != widget.SceneVersion {
		t.Fatalf("GetWidget: %+v, %v", w, err)
	}
	updatedWidget, err := client.UpdateWidget(ctx, scene.ID, widget.ID, apiv0.UpdateWidgetRequest{
		Name: "lamp2", Size: apiv0.SizeRequest{Width: 10, Height: 10}, Origin: apiv0.OriginRequest{X: 0.5, Y: 0.5},
		TypeID: wt.ID, SceneVersion: widget.SceneVersion, Labels: []string{}, PortBindings: []apiv0.PortBinding{},
	})
	if err != nil || updatedWidget.SceneVersion <= widget.SceneVersion {
		t.Fatalf("UpdateWidget: %+v, %v", updatedWidget, err)
	}
	widgets, err := client.ListWidgets(ctx, scene.ID)
	if err != nil || len(widgets) != 1 || widgets[0].Name != "lamp2" {
		t.Fatalf("ListWidgets: %+v, %v", widgets, err)
	}
	deletedWidget, err := client.DeleteWidget(ctx, scene.ID, widget.ID)
	if err != nil || deletedWidget.ID != widget.ID || deletedWidget.SceneVersion <= updatedWidget.SceneVersion {
		t.Fatalf("DeleteWidget: %+v, %v", deletedWidget, err)
	}

	scenes, err := client.ListScenes(ctx)
	if err != nil || len(scenes) != 1 || scenes[0].Version != deletedWidget.SceneVersion {
		t.Fatalf("ListScenes: %+v, %v", scenes, err)
	}
	if deleted, err := client.DeleteScene(ctx, scene.ID); err != nil || deleted.ID != scene.ID {
		t.Fatalf("DeleteScene: %+v, %v", deleted, err)
	}
	if _, err := client.GetScene(ctx, scene.ID); !errors.Is(err, apiclient.ErrNotFound) {
		t.Errorf("GetScene after delete: %v, want ErrNotFound", err)
	}
}

func TestBadRequest_CarriesTheServersDetail(t *testing.T) {
	client := startClient(t)

	_, err := client.CreateTag(context.Background(), apiv0.CreateTagRequest{Name: "x", Type: "complex", Value: "", Quality: "good"})

	var se *apiclient.StatusError
	if !errors.As(err, &se) || se.Problem.Status != http.StatusBadRequest || se.Problem.Title != "Bad Request" || se.Problem.Detail == "" {
		t.Fatalf("got %v, want a 400 with the server's detail", err)
	}
}
