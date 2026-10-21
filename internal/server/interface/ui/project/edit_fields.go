package project

import (
	"fmt"
	"strconv"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Editor fields ─────────────────────────────────────────────────────────────
//
// The properties editors hold text. A fresh server state is merged into them
// field by field: a field that still holds the old server value follows the
// new one, an unsaved edit is kept.

// ── Widget ────────────────────────────────────────────────────────────────────

// widgetEditFields is the widget properties editor's text, formatted as the
// editor shows it.
type widgetEditFields struct {
	name, posX, posY, posZ, width, height, originX, originY, rotation string
}

func widgetEditFieldsOf(w apiv0.WidgetResponse) widgetEditFields {
	return widgetEditFields{
		name:     w.Name,
		posX:     fmt.Sprintf("%.1f", w.Position.X),
		posY:     fmt.Sprintf("%.1f", w.Position.Y),
		posZ:     strconv.Itoa(w.Position.Z),
		width:    strconv.Itoa(w.Size.Width),
		height:   strconv.Itoa(w.Size.Height),
		originX:  fmt.Sprintf("%.3f", w.Origin.X),
		originY:  fmt.Sprintf("%.3f", w.Origin.Y),
		rotation: fmt.Sprintf("%.1f", w.Rotation.Degrees),
	}
}

func mergeWidgetEditFields(local, old, current widgetEditFields) widgetEditFields {
	m := uiutil.MergeField[string]
	return widgetEditFields{
		name:     m(local.name, old.name, current.name),
		posX:     m(local.posX, old.posX, current.posX),
		posY:     m(local.posY, old.posY, current.posY),
		posZ:     m(local.posZ, old.posZ, current.posZ),
		width:    m(local.width, old.width, current.width),
		height:   m(local.height, old.height, current.height),
		originX:  m(local.originX, old.originX, current.originX),
		originY:  m(local.originY, old.originY, current.originY),
		rotation: m(local.rotation, old.rotation, current.rotation),
	}
}

func (p *Project) syncEditingFields(w apiv0.WidgetResponse) {
	p.widgetEdit = widgetEditFieldsOf(w)
}

// mergeWidgetEditingFields updates each widget property field that still holds
// the old server value.
func (p *Project) mergeWidgetEditingFields(old, current apiv0.WidgetResponse) {
	p.widgetEdit = mergeWidgetEditFields(p.widgetEdit, widgetEditFieldsOf(old), widgetEditFieldsOf(current))
}

// ── Scene ─────────────────────────────────────────────────────────────────────

// sceneEditFields is the scene properties editor's text, formatted as the
// editor shows it.
type sceneEditFields struct {
	name, width, height, background string
}

func sceneEditFieldsOf(sc apiv0.SceneResponse) sceneEditFields {
	return sceneEditFields{
		name:       sc.Name,
		width:      strconv.Itoa(sc.Width),
		height:     strconv.Itoa(sc.Height),
		background: sc.BackgroundHTML,
	}
}

func mergeSceneEditFields(local, old, current sceneEditFields) sceneEditFields {
	m := uiutil.MergeField[string]
	return sceneEditFields{
		name:       m(local.name, old.name, current.name),
		width:      m(local.width, old.width, current.width),
		height:     m(local.height, old.height, current.height),
		background: m(local.background, old.background, current.background),
	}
}

// syncSceneEditingFields populates the Properties-panel editing state from
// the currently selected scene; with none, empties it.
func (p *Project) syncSceneEditingFields() {
	sc, ok := p.findScene(p.selectedSceneID)
	if !ok {
		p.sceneEdit = sceneEditFields{}
		return
	}
	p.sceneEdit = sceneEditFieldsOf(sc)
}

// mergeSceneEditingFields updates each scene property field that still holds
// the old server value.
func (p *Project) mergeSceneEditingFields(old, current apiv0.SceneResponse) {
	p.sceneEdit = mergeSceneEditFields(p.sceneEdit, sceneEditFieldsOf(old), sceneEditFieldsOf(current))
}
