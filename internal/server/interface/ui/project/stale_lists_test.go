package project

import (
	"slices"
	"testing"
)

// The lists a mode reloads can be overtaken by the writes it makes: a list
// requested before a creation, a rename or a switch of Scene is older than
// what the mode already knows, and must not undo it (task 61). Each test
// takes a list — once loaded, a copy of it — then writes, then feeds the old
// list through the loader the way a late response arrives.

func TestCreateScene_AListSentBefore_KeepsTheSceneAndTheSelection(t *testing.T) {
	f := startProject(t)
	stale := slices.Clone(f.p.scenes) // a list in flight, before the Scene
	mark := f.p.draftVersions.Mark()

	f.p.createScene(f.p.compoCtx)
	f.run()
	created := f.p.selectedSceneID
	f.p.setScenes(f.p.compoCtx, mark, f.p.contentReplacements, stale)

	if len(f.p.scenes) != 1 || f.p.scenes[0].ID != created {
		t.Fatalf("scenes %+v: the new one %v is gone", f.p.scenes, created)
	}
	if f.p.selectedSceneID != created {
		t.Errorf("selected %v, want the new Scene %v", f.p.selectedSceneID, created)
	}
}

func TestCreateWidget_AListSentBefore_KeepsTheWidgetAndTheSelection(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	typeID := createWidgetType(t, api, "Lamp")
	f := mount(t, url, api)
	f.run()
	stale := slices.Clone(f.p.widgets) // a list in flight, before the Widget
	mark := f.p.draftVersions.Mark()

	f.p.createWidget(f.p.compoCtx, typeID, 10, 20)
	f.run()
	created := f.p.primaryWidgetID()
	f.p.setWidgets(f.p.compoCtx, mark, sceneID, stale)

	if len(f.p.widgets) != 1 || f.p.widgets[0].ID != created {
		t.Fatalf("widgets %+v: the new one %v is gone", f.p.widgets, created)
	}
	if f.p.primaryWidgetID() != created {
		t.Errorf("selected %v, want the new Widget %v", f.p.primaryWidgetID(), created)
	}
}

// A list of a Scene no longer shown cannot put its Widgets on the canvas: the
// selection moved while the response was on its way.
func TestSetWidgets_AListOfAnotherScene_IsDropped(t *testing.T) {
	url, api := newServer(t)
	first := createScene(t, api, "First")
	second := createScene(t, api, "Second")
	typeID := createWidgetType(t, api, "Lamp")
	createWidget(t, api, first, typeID, "theirs")
	f := mount(t, url, api)
	f.run()

	mark := f.p.draftVersions.Mark()
	stale := slices.Clone(f.p.widgets) // the Widgets of the first Scene
	stale[0].Name = "stale"
	f.p.selectScene(f.p.compoCtx, second)
	f.p.setWidgets(f.p.compoCtx, mark, first, stale)

	if len(f.p.widgets) != 1 || f.p.widgets[0].Name != "theirs" {
		t.Errorf("widgets %+v of the Scene left: want the canvas untouched", f.p.widgets)
	}
}

func TestScenesList_AnsweredAfterARename_DoesNotRollTheNameBack(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)
	f.run()
	stale := slices.Clone(f.p.scenes) // a list in flight, before the rename
	mark := f.p.draftVersions.Mark()

	f.p.startEditingScene(sceneID, "Main")
	f.p.editingSceneName = "Renamed"
	f.p.commitSceneEdit(f.p.compoCtx)
	f.run()
	f.p.setScenes(f.p.compoCtx, mark, f.p.contentReplacements, stale)

	if name := f.p.scenes[0].Name; name != "Renamed" {
		t.Errorf("listed name %q: the stale list rolled it back", name)
	}
	if name := f.p.sceneEdit.name; name != "Renamed" {
		t.Errorf("editor name %q: the stale list rolled it back", name)
	}
	f.knowsVersion(sceneID)
}
