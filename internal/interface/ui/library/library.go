package library

import (
	"encoding/json"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Component ─────────────────────────────────────────────────────────────────

type Library struct {
	app.Compo
	apiServerURL     string
	widgetTypes      []widgetTypeItem
	loading          bool
	fetchErr         string
	selectedID       string
	editedName       string
	editedHTML       string
	editedScript     string
	editedInputData  string
	editedScriptLang string
	editingID        string
	editingName      string
}

func NewLibrary(apiServerURL string) *Library {
	return &Library{apiServerURL: apiServerURL}
}

func (l *Library) OnMount(ctx app.Context) {
	l.loadList(ctx)
}

// ── Data loading ──────────────────────────────────────────────────────────────

func (l *Library) loadList(ctx app.Context) {
	l.loading = true
	url := l.apiServerURL + "/api/v1/widget-types"
	ctx.Async(func() {
		resp, err := http.Get(url)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				l.loading = false
				l.fetchErr = err.Error()
			})
			return
		}
		defer resp.Body.Close()

		var result getWidgetTypesResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				l.loading = false
				l.fetchErr = err.Error()
			})
			return
		}

		ctx.Dispatch(func(ctx app.Context) {
			l.loading = false
			l.widgetTypes = result.WidgetTypes
		})
	})
}

// ── Selection helpers ─────────────────────────────────────────────────────────

func (l *Library) selectItem(id string) {
	for _, it := range l.widgetTypes {
		if it.ID == id {
			l.selectedID = id
			l.editedName = it.Name
			l.editedHTML = it.HtmlTemplate
			l.editedScript = it.Script
			l.editedScriptLang = it.ScriptLanguage
			l.editedInputData = ""
			return
		}
	}
}

func (l *Library) startEditing(id, currentName string) {
	l.editingID = id
	l.editingName = currentName
}

// ── Render ────────────────────────────────────────────────────────────────────

func (l *Library) Render() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex", "1").
		Style("min-height", "0").
		Style("gap", "8px").
		Body(
			l.renderListColumn(),
			l.renderEditorColumn("HTML Template", "html-template", l.editedHTML, func(ctx app.Context, e app.Event) {
				l.editedHTML = ctx.JSSrc().Get("value").String()
			}, true),
			l.renderEditorColumn("Script", "script-editor", l.editedScript, func(ctx app.Context, e app.Event) {
				l.editedScript = ctx.JSSrc().Get("value").String()
			}, true),
			l.renderEditorColumn("Input Data", "input-data", l.editedInputData, func(ctx app.Context, e app.Event) {
				l.editedInputData = ctx.JSSrc().Get("value").String()
			}, false),
			l.renderPreviewColumn(),
		)
}
