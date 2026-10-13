package library

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

const defaultHTML = `<div class="widget"></div>`
const defaultScript = "function update() {\n}"

// ── Widget type CRUD ──────────────────────────────────────────────────────────

func (l *Library) createItem(ctx app.Context) {
	ctx = l.compoCtx
	req := apiv0.CreateWidgetTypeRequest{
		Name:           "New Widget Type",
		HtmlTemplate:   defaultHTML,
		Script:         defaultScript,
		ScriptLanguage: "javascript",
		DefaultWidth:   120,
		DefaultHeight:  60,
		InputPorts:     []apiv0.InputPort{},
	}
	uiutil.Call(ctx, func(c context.Context) (apiv0.CreateWidgetTypeResponse, error) {
		return l.client.CreateWidgetType(c, req)
	}, func(ctx app.Context, created apiv0.CreateWidgetTypeResponse) {
		l.selectedID = created.ID
		l.editedName = req.Name
		l.editedHTML = req.HtmlTemplate
		l.editedScript = req.Script
		l.editedInputPorts = nil
		l.editedScriptLang = req.ScriptLanguage
		l.loadList(ctx)
	}, nil)
}

func (l *Library) deleteItem(ctx app.Context) {
	ctx = l.compoCtx
	if l.selectedID == uuid.Nil {
		return
	}
	deletedID := l.selectedID
	uiutil.Call(ctx, func(c context.Context) (apiv0.WidgetTypeResponse, error) {
		return l.client.DeleteWidgetType(c, deletedID)
	}, func(ctx app.Context, _ apiv0.WidgetTypeResponse) {
		if l.selectedID == deletedID {
			l.clearSelection()
		}
		l.loadList(ctx)
	}, nil)
}

func (l *Library) applyChanges(ctx app.Context) {
	if l.selectedID == uuid.Nil {
		return
	}
	current, _ := l.findItem(l.selectedID)
	ports := l.editedInputPorts
	if ports == nil {
		ports = []apiv0.InputPort{}
	}
	l.updateWidgetType(l.selectedID, apiv0.UpdateWidgetTypeRequest{
		Name:           l.editedName,
		HtmlTemplate:   l.editedHTML,
		Script:         l.editedScript,
		ScriptLanguage: l.editedScriptLang,
		DefaultWidth:   current.DefaultWidth,
		DefaultHeight:  current.DefaultHeight,
		InputPorts:     ports,
		Version:        current.Version,
	})
}

func (l *Library) commitEdit(ctx app.Context) {
	if l.editingID == uuid.Nil {
		return
	}
	id := l.editingID
	name := l.editingName
	l.editingID = uuid.Nil
	l.editingName = ""

	found, ok := l.findItem(id)
	if !ok {
		return
	}
	for i := range l.widgetTypes {
		if l.widgetTypes[i].ID == id {
			l.widgetTypes[i].Name = name
			break
		}
	}
	if l.selectedID == id {
		l.editedName = name
	}

	ports := found.InputPorts
	if ports == nil {
		ports = []apiv0.InputPort{}
	}
	l.updateWidgetType(id, apiv0.UpdateWidgetTypeRequest{
		Name:           name,
		HtmlTemplate:   found.HtmlTemplate,
		Script:         found.Script,
		ScriptLanguage: found.ScriptLanguage,
		DefaultWidth:   found.DefaultWidth,
		DefaultHeight:  found.DefaultHeight,
		InputPorts:     ports,
		Version:        found.Version,
	})
}

// updateWidgetType saves the WidgetType, then records its new version and
// reloads the list.
func (l *Library) updateWidgetType(id uuid.UUID, req apiv0.UpdateWidgetTypeRequest) {
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.UpdateWidgetTypeResponse, error) {
		return l.client.UpdateWidgetType(c, id, req)
	}, func(ctx app.Context, updated apiv0.UpdateWidgetTypeResponse) {
		for i := range l.widgetTypes {
			if l.widgetTypes[i].ID == id {
				l.widgetTypes[i].Version = updated.Version
				break
			}
		}
		l.loadList(ctx)
	}, nil)
}
