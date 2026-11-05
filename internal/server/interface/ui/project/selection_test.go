package project

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// spreadWidgets puts the Widgets at x = 10, 210, 410, … and y = 20, so a
// marquee or a group move can tell them apart.
func spreadWidgets(t *testing.T, api *testAPI, sceneID uuid.UUID, ids []uuid.UUID) {
	t.Helper()
	for i, id := range ids {
		ctx := context.Background()
		sc, err := api.GetScene(ctx, sceneID)
		if err != nil {
			t.Fatal(err)
		}
		w, err := api.GetWidget(ctx, sceneID, id)
		if err != nil {
			t.Fatal(err)
		}
		req := widgetRequest(w, sc.Version)
		req.Position = apiv0.PositionRequest{X: 10 + 200*float64(i), Y: 20}
		if _, err := api.UpdateWidget(ctx, sceneID, id, req); err != nil {
			t.Fatal(err)
		}
	}
}

// selectionFixture is a Project showing a Scene with n Widgets at x = 10, 210,
// 410 and y = 20, loaded and selected by nobody.
func selectionFixture(t *testing.T, n int) (*fixture, uuid.UUID, []uuid.UUID) {
	t.Helper()
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	typeID := createWidgetType(t, api, "Gauge")
	ids := make([]uuid.UUID, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, createWidget(t, api, sceneID, typeID, "gauge"+string(rune('a'+i))))
	}
	spreadWidgets(t, api, sceneID, ids)
	f := mount(t, url, api)
	f.run()
	f.p.clearWidgetSelection(f.p.compoCtx)
	return f, sceneID, ids
}

func (f *fixture) selection() []uuid.UUID { return f.p.selectedWidgetIDs }

func TestSelectOnlyAndClear(t *testing.T) {
	f, _, ids := selectionFixture(t, 2)

	f.p.selectOnly(f.p.compoCtx, ids[0])
	if got := f.selection(); len(got) != 1 || got[0] != ids[0] {
		t.Errorf("selectOnly: %v", got)
	}
	if f.p.primaryWidgetID() != ids[0] {
		t.Errorf("primary %v, want %v", f.p.primaryWidgetID(), ids[0])
	}
	if f.p.widgetEdit.name == "" {
		t.Error("the single selection did not fill the properties editor")
	}

	f.p.clearWidgetSelection(f.p.compoCtx)
	if got := f.selection(); len(got) != 0 {
		t.Errorf("clear: %v", got)
	}
	if f.p.primaryWidgetID() != uuid.Nil {
		t.Errorf("primary %v after clear", f.p.primaryWidgetID())
	}
	if f.p.widgetEdit != (widgetEditFields{}) {
		t.Errorf("the editor was not emptied: %+v", f.p.widgetEdit)
	}
}

func TestToggleWidgetSelection_AddsAndRemovesKeepingTheLead(t *testing.T) {
	f, _, ids := selectionFixture(t, 3)

	f.p.toggleWidgetSelection(f.p.compoCtx, ids[0])
	f.p.toggleWidgetSelection(f.p.compoCtx, ids[2])
	if got := f.selection(); len(got) != 2 || got[0] != ids[0] || got[1] != ids[2] {
		t.Fatalf("added: %v", got)
	}
	if f.p.primaryWidgetID() != ids[2] {
		t.Errorf("primary %v, want the last added %v", f.p.primaryWidgetID(), ids[2])
	}

	f.p.toggleWidgetSelection(f.p.compoCtx, ids[2])
	if got := f.selection(); len(got) != 1 || got[0] != ids[0] {
		t.Errorf("removed the lead: %v", got)
	}
	if f.p.primaryWidgetID() != ids[0] {
		t.Errorf("primary %v, want the last remaining %v", f.p.primaryWidgetID(), ids[0])
	}
}

func TestAddToSelectionKeepsOrderAndDoesNotRepeat(t *testing.T) {
	f, _, ids := selectionFixture(t, 3)

	f.p.addToSelection(f.p.compoCtx, []uuid.UUID{ids[2], ids[0], ids[2]})
	if got := f.selection(); len(got) != 2 || got[0] != ids[2] || got[1] != ids[0] {
		t.Errorf("add: %v", got)
	}
	if f.p.primaryWidgetID() != ids[0] {
		t.Errorf("primary %v, want %v", f.p.primaryWidgetID(), ids[0])
	}
}

func TestSelectAllWidgets_SelectsInSceneOrder(t *testing.T) {
	f, _, ids := selectionFixture(t, 3)

	f.p.selectAllWidgets(f.p.compoCtx)
	if got := f.selection(); len(got) != 3 || got[0] != ids[0] || got[1] != ids[1] || got[2] != ids[2] {
		t.Errorf("select all: %v", got)
	}
	if f.p.primaryWidgetID() != ids[2] {
		t.Errorf("primary %v, want the last of the Scene %v", f.p.primaryWidgetID(), ids[2])
	}
}

func TestEscapeDropsTheSelection(t *testing.T) {
	f, _, _ := selectionFixture(t, 2)
	f.p.selectAllWidgets(f.p.compoCtx)

	f.p.escapeSelection(f.p.compoCtx)

	if len(f.selection()) != 0 || f.p.primaryWidgetID() != uuid.Nil {
		t.Errorf("selection after Esc: %v", f.selection())
	}
}

func TestSetWidgets_PrunesGoneWidgetsAndMovesTheLead(t *testing.T) {
	f, sceneID, ids := selectionFixture(t, 3)
	f.p.selectAllWidgets(f.p.compoCtx)

	ctx := context.Background()
	sc, err := f.api.GetScene(ctx, sceneID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.api.DeleteWidget(ctx, sceneID, ids[2], sc.Version); err != nil {
		t.Fatal(err)
	}
	f.p.loadWidgets(f.p.compoCtx)
	f.run()

	if got := f.selection(); len(got) != 2 || got[0] != ids[0] || got[1] != ids[1] {
		t.Errorf("selection after the delete: %v", got)
	}
	if f.p.primaryWidgetID() != ids[1] {
		t.Errorf("primary %v, want the last remaining %v", f.p.primaryWidgetID(), ids[1])
	}
}

func TestDeleteWidget_RemovesOnlyThatWidgetFromTheSelection(t *testing.T) {
	f, _, ids := selectionFixture(t, 2)
	f.p.selectAllWidgets(f.p.compoCtx)

	f.p.deleteWidget(f.p.compoCtx, ids[1])
	f.run()

	f.noToasts()
	if got := f.selection(); len(got) != 1 || got[0] != ids[0] {
		t.Errorf("selection after deleting %v: %v", ids[1], got)
	}
}

func TestPropertiesPanel_SeveralSelectedShowsTheSummary(t *testing.T) {
	f, _, ids := selectionFixture(t, 2)
	f.p.selectAllWidgets(f.p.compoCtx)

	html := app.HTMLString(f.p.renderPropertiesPanel())
	if !contains(html, "2 widgets selected") {
		t.Errorf("the summary is missing: %s", html)
	}
	for _, unwanted := range []string{"Delete Widget", "Position (px)", "Port Bindings"} {
		if contains(html, unwanted) {
			t.Errorf("a group selection shows %q: %s", unwanted, html)
		}
	}
	if f.p.selectedWidgetIdx() != -1 {
		t.Errorf("selectedWidgetIdx %d with two selected: want -1", f.p.selectedWidgetIdx())
	}

	f.p.selectOnly(f.p.compoCtx, ids[0])
	if !contains(app.HTMLString(f.p.renderPropertiesPanel()), "Delete Widget") {
		t.Error("the single selection has no editors")
	}
}

func TestPropertiesPanel_NoSelectionShowsTheScene(t *testing.T) {
	f, _, _ := selectionFixture(t, 1)
	f.p.clearWidgetSelection(f.p.compoCtx)

	html := app.HTMLString(f.p.renderPropertiesPanel())
	if contains(html, "Delete Widget") {
		t.Errorf("no selection shows widget editors: %s", html)
	}
	if !contains(html, "Background HTML") {
		t.Errorf("no selection does not show the Scene: %s", html)
	}
}

func TestRestoreSelection_KeepsOnlyThePrimary(t *testing.T) {
	f, _, ids := selectionFixture(t, 2)
	f.p.selectAllWidgets(f.p.compoCtx)
	f.p.compoCtx.LocalStorage().Set(f.p.selectionKey(widgetIDKey), ids[1].String())

	f.p.restoreSelection(f.p.compoCtx)

	if got := f.selection(); len(got) != 1 || got[0] != ids[1] {
		t.Errorf("restored selection: %v, want the primary %v alone", got, ids[1])
	}
}

// contains is strings.Contains under the name the assertions read best with.
func contains(s, sub string) bool { return strings.Contains(s, sub) }
