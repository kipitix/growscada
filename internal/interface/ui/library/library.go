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
	editedName        string
	editedHTML        string
	editedScript      string
	editedInputValues map[string]string
	editedScriptLang  string
	editedInputPorts  []inputPortDTO
	editingID         string
	editingName       string
	// add-port form state
	newPortName string
	newPortDesc string
	newPortType string
}

func NewLibrary(apiServerURL string) *Library {
	return &Library{apiServerURL: apiServerURL}
}

func (l *Library) OnMount(ctx app.Context) {
	ctx.LocalStorage().Get("library:selectedID", &l.selectedID)
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
			// Restore selection: re-run selectItem to populate editing fields.
			if l.selectedID != "" {
				found := false
				for _, it := range result.WidgetTypes {
					if it.ID == l.selectedID {
						found = true
						break
					}
				}
				if found {
					l.selectItem(l.selectedID)
				} else {
					l.selectedID = ""
					ctx.LocalStorage().Set("library:selectedID", "")
				}
			}
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
			l.editedInputValues = make(map[string]string)
			ports := make([]inputPortDTO, len(it.InputPorts))
			copy(ports, it.InputPorts)
			l.editedInputPorts = ports
			l.newPortName = ""
			l.newPortDesc = ""
			l.newPortType = ""
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
			l.renderInputPortsColumn(),
			l.renderInputDataColumn(),
			l.renderPreviewColumn(),
		)
}
