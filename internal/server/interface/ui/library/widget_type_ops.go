package library

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

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
	mark := l.versions.Mark()
	uiutil.Call(ctx, func(c context.Context) (apiv0.CreateWidgetTypeResponse, error) {
		return l.client.CreateWidgetType(c, req)
	}, func(ctx app.Context, created apiv0.CreateWidgetTypeResponse) {
		// The new type is the request as the server stored it: listed and
		// editable at once, before the list is reread.
		l.versions.Observe(mark, created.ID, created.Version)
		l.widgetTypes = append(l.widgetTypes, apiv0.WidgetTypeResponse{
			ID:             created.ID,
			Name:           req.Name,
			HtmlTemplate:   req.HtmlTemplate,
			Script:         req.Script,
			ScriptLanguage: req.ScriptLanguage,
			DefaultWidth:   req.DefaultWidth,
			DefaultHeight:  req.DefaultHeight,
			InputPorts:     req.InputPorts,
			Version:        created.Version,
		})
		l.selectItem(created.ID)
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
	current, ok := l.findItem(l.selectedID)
	if !ok {
		return
	}
	l.updateWidgetType(l.selectedID, apiv0.UpdateWidgetTypeRequest{
		Name:           l.edit.name,
		HtmlTemplate:   l.edit.html,
		Script:         l.edit.script,
		ScriptLanguage: l.edit.scriptLang,
		DefaultWidth:   current.DefaultWidth,
		DefaultHeight:  current.DefaultHeight,
		InputPorts:     portsRequest(l.edit.ports),
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
		l.edit.name = name
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
// reloads the list. On a conflict (409) its known version is forgotten and
// the list reread: if the server's version went back, a newer one kept here
// would be rejected forever. The other types' versions stay.
func (l *Library) updateWidgetType(id uuid.UUID, req apiv0.UpdateWidgetTypeRequest) {
	mark := l.versions.Mark()
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.UpdateWidgetTypeResponse, error) {
		return l.client.UpdateWidgetType(c, id, req)
	}, func(ctx app.Context, updated apiv0.UpdateWidgetTypeResponse) {
		l.versions.Observe(mark, id, updated.Version)
		l.loadList(ctx)
	}, func(ctx app.Context, err error) {
		uiutil.ShowError(ctx, err)
		if errors.Is(err, apiclient.ErrConflict) {
			l.versions.ForgetID(id)
			l.loadList(ctx)
		}
	})
}
