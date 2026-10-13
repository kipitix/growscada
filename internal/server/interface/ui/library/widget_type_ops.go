package library

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
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
	l.updateWidgetType(l.selectedID, apiv0.UpdateWidgetTypeRequest{
		Name:           l.editedName,
		HtmlTemplate:   l.editedHTML,
		Script:         l.editedScript,
		ScriptLanguage: l.editedScriptLang,
		DefaultWidth:   current.DefaultWidth,
		DefaultHeight:  current.DefaultHeight,
		InputPorts:     portsRequest(l.editedInputPorts),
		Version:        l.versions.Of(l.selectedID),
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

	idx := l.itemIdx(id)
	if idx < 0 {
		return
	}
	l.widgetTypes[idx].Name = name
	found := l.widgetTypes[idx]
	if l.selectedID == id {
		l.editedName = name
	}

	l.updateWidgetType(id, apiv0.UpdateWidgetTypeRequest{
		Name:           name,
		HtmlTemplate:   found.HtmlTemplate,
		Script:         found.Script,
		ScriptLanguage: found.ScriptLanguage,
		DefaultWidth:   found.DefaultWidth,
		DefaultHeight:  found.DefaultHeight,
		InputPorts:     portsRequest(found.InputPorts),
		Version:        l.versions.Of(id),
	})
}

// portsRequest returns a copy of ports, never nil: the body is marshalled later,
// on the Async goroutine, while the editor may already change its ports.
func portsRequest(ports []apiv0.InputPort) []apiv0.InputPort {
	return append([]apiv0.InputPort{}, ports...)
}

// updateWidgetType saves the WidgetType, then records its new version and
// reloads the list. On a conflict (409) the known versions are forgotten and
// the list reread: if the server's version went back, a newer one kept here
// would be rejected forever.
func (l *Library) updateWidgetType(id uuid.UUID, req apiv0.UpdateWidgetTypeRequest) {
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.UpdateWidgetTypeResponse, error) {
		return l.client.UpdateWidgetType(c, id, req)
	}, func(ctx app.Context, updated apiv0.UpdateWidgetTypeResponse) {
		l.versions.Observe(id, updated.Version)
		l.loadList(ctx)
	}, func(ctx app.Context, err error) {
		uiutil.ShowError(ctx, err)
		if errors.Is(err, apiclient.ErrConflict) {
			l.versions.Forget()
			l.loadList(ctx)
		}
	})
}
