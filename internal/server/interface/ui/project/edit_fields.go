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

func (p *Project) widgetEditFields() widgetEditFields {
	return widgetEditFields{
		name:     p.editingWidgetName,
		posX:     p.editingPosX,
		posY:     p.editingPosY,
		posZ:     p.editingPosZ,
		width:    p.editingWidth,
		height:   p.editingHeight,
		originX:  p.editingOriginX,
		originY:  p.editingOriginY,
		rotation: p.editingRotation,
	}
}

func (p *Project) setWidgetEditFields(f widgetEditFields) {
	p.editingWidgetName = f.name
	p.editingPosX = f.posX
	p.editingPosY = f.posY
	p.editingPosZ = f.posZ
	p.editingWidth = f.width
	p.editingHeight = f.height
	p.editingOriginX = f.originX
	p.editingOriginY = f.originY
	p.editingRotation = f.rotation
}

func (p *Project) syncEditingFields(w apiv0.WidgetResponse) {
	p.setWidgetEditFields(widgetEditFieldsOf(w))
}

// mergeWidgetEditingFields updates each widget property field that still holds
// the old server value.
func (p *Project) mergeWidgetEditingFields(old, current apiv0.WidgetResponse) {
	p.setWidgetEditFields(mergeWidgetEditFields(p.widgetEditFields(), widgetEditFieldsOf(old), widgetEditFieldsOf(current)))
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

func (p *Project) sceneEditFields() sceneEditFields {
	return sceneEditFields{
		name:       p.editingScenePropsName,
		width:      p.editingScenePropsWidth,
		height:     p.editingScenePropsHeight,
		background: p.editingScenePropsBG,
	}
}

func (p *Project) setSceneEditFields(f sceneEditFields) {
	p.editingScenePropsName = f.name
	p.editingScenePropsWidth = f.width
	p.editingScenePropsHeight = f.height
	p.editingScenePropsBG = f.background
}

// syncSceneEditingFields populates the Properties-panel editing state from
// the currently selected scene; with none, empties it.
func (p *Project) syncSceneEditingFields() {
	sc, ok := p.findScene(p.selectedSceneID)
	if !ok {
		p.setSceneEditFields(sceneEditFields{})
		return
	}
	p.setSceneEditFields(sceneEditFieldsOf(sc))
}

// mergeSceneEditingFields updates each scene property field that still holds
// the old server value.
func (p *Project) mergeSceneEditingFields(old, current apiv0.SceneResponse) {
	p.setSceneEditFields(mergeSceneEditFields(p.sceneEditFields(), sceneEditFieldsOf(old), sceneEditFieldsOf(current)))
}
