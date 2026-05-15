package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

type widgetTypeItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	HtmlTemplate   string `json:"html_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
}

type getWidgetTypesResponse struct {
	WidgetTypes []widgetTypeItem `json:"widget_types"`
}

type createWidgetTypeRequest struct {
	Name           string `json:"name"`
	HtmlTemplate   string `json:"html_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
}

type createWidgetTypeResponse struct {
	ID string `json:"id"`
}

type updateWidgetTypeRequest struct {
	Name           string `json:"name"`
	HtmlTemplate   string `json:"html_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
}

type Library struct {
	app.Compo
	apiServerURL   string
	widgetTypes    []widgetTypeItem
	loading        bool
	fetchErr       string
	selectedID       string
	editedName       string
	editedHTML       string
	editedScript     string
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

const defaultHTML = `<div class="widget"></div>`
const defaultScript = "function update() {\n}"

func (l *Library) createItem(ctx app.Context) {
	name := "New Widget Type"
	url := l.apiServerURL + "/api/v1/widget-types"
	body, _ := json.Marshal(createWidgetTypeRequest{
		Name:           name,
		HtmlTemplate:   defaultHTML,
		Script:         defaultScript,
		ScriptLanguage: "javascript",
	})
	ctx.Async(func() {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				l.fetchErr = err.Error()
			})
			return
		}
		defer resp.Body.Close()

		var result createWidgetTypeResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				l.fetchErr = err.Error()
			})
			return
		}

		newID := result.ID
		ctx.Dispatch(func(ctx app.Context) {
			l.fetchErr = ""
			l.selectedID = newID
			l.editedName = name
			l.editedHTML = defaultHTML
			l.editedScript = defaultScript
			l.editedScriptLang = "javascript"
			l.loadList(ctx)
		})
	})
}

func (l *Library) deleteItem(ctx app.Context) {
	if l.selectedID == "" {
		return
	}
	url := l.apiServerURL + "/api/v1/widget-types/" + l.selectedID
	deletedID := l.selectedID
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodDelete, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				l.fetchErr = err.Error()
			})
			return
		}
		resp.Body.Close()

		ctx.Dispatch(func(ctx app.Context) {
			l.fetchErr = ""
			if l.selectedID == deletedID {
				l.selectedID = ""
				l.editedHTML = ""
				l.editedScript = ""
			}
			l.loadList(ctx)
		})
	})
}

func (l *Library) selectItem(id string) {
	for _, it := range l.widgetTypes {
		if it.ID == id {
			l.selectedID = id
			l.editedName = it.Name
			l.editedHTML = it.HtmlTemplate
			l.editedScript = it.Script
			l.editedScriptLang = it.ScriptLanguage
			return
		}
	}
}

func (l *Library) applyChanges(ctx app.Context) {
	if l.selectedID == "" {
		return
	}
	url := l.apiServerURL + "/api/v1/widget-types/" + l.selectedID
	body, _ := json.Marshal(updateWidgetTypeRequest{
		Name:           l.editedName,
		HtmlTemplate:   l.editedHTML,
		Script:         l.editedScript,
		ScriptLanguage: l.editedScriptLang,
	})
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				l.fetchErr = err.Error()
			})
			return
		}
		resp.Body.Close()
		ctx.Dispatch(func(ctx app.Context) {
			l.fetchErr = ""
			l.loadList(ctx)
		})
	})
}

func (l *Library) startEditing(id, currentName string) {
	l.editingID = id
	l.editingName = currentName
}

func (l *Library) commitEdit(ctx app.Context) {
	if l.editingID == "" {
		return
	}
	id := l.editingID
	name := l.editingName
	l.editingID = ""
	l.editingName = ""

	var found widgetTypeItem
	for _, it := range l.widgetTypes {
		if it.ID == id {
			found = it
			break
		}
	}
	if found.ID == "" {
		return
	}

	url := l.apiServerURL + "/api/v1/widget-types/" + id
	body, _ := json.Marshal(updateWidgetTypeRequest{
		Name:           name,
		HtmlTemplate:   found.HtmlTemplate,
		Script:         found.Script,
		ScriptLanguage: found.ScriptLanguage,
	})
	ctx.Async(func() {
		req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				l.fetchErr = err.Error()
			})
			return
		}
		resp.Body.Close()
		ctx.Dispatch(func(ctx app.Context) {
			l.fetchErr = ""
			if l.selectedID == id {
				l.editedName = name
			}
			l.loadList(ctx)
		})
	})
}

func (l *Library) Render() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("flex", "1").
		Style("min-height", "0").
		Style("gap", "8px").
		Body(
			l.renderListColumn(),
			l.renderEditorColumn("HTML Template", l.editedHTML, func(ctx app.Context, e app.Event) {
				l.editedHTML = ctx.JSSrc().Get("value").String()
			}, true),
			l.renderEditorColumn("Script", l.editedScript, func(ctx app.Context, e app.Event) {
				l.editedScript = ctx.JSSrc().Get("value").String()
			}, true),
			l.renderEditorColumn("Input Data", "", nil, false),
			l.renderPreviewColumn(),
		)
}

func (l *Library) renderListColumn() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("width", "200px").
		Style("flex-shrink", "0").
		Style("min-height", "0").
		Body(
			app.H3().Style("margin", "0 0 8px 0").Text("Widget Types"),
			l.renderListButtons(),
			app.Div().
				Style("flex", "1").
				Style("overflow-y", "auto").
				Style("margin-top", "4px").
				Body(l.renderList()),
		)
}

func (l *Library) renderListButtons() app.UI {
	deleteDisabled := l.selectedID == ""

	deleteBtn := app.Button().
		Style("flex", "1").
		Style("padding", "4px 0").
		Style("font-size", "13px").
		Style("cursor", "pointer").
		Style("border", "1px solid #ccc").
		Style("border-radius", "4px").
		Text("Delete").
		OnClick(func(ctx app.Context, e app.Event) {
			l.deleteItem(ctx)
		})
	if deleteDisabled {
		deleteBtn = deleteBtn.
			Style("opacity", "0.4").
			Style("cursor", "default").
			Disabled(true)
	}

	return app.Div().
		Style("display", "flex").
		Style("gap", "4px").
		Style("margin-bottom", "4px").
		Body(
			app.Button().
				Style("flex", "1").
				Style("padding", "4px 0").
				Style("font-size", "13px").
				Style("cursor", "pointer").
				Style("border", "1px solid #ccc").
				Style("border-radius", "4px").
				Text("Create").
				OnClick(func(ctx app.Context, e app.Event) {
					l.createItem(ctx)
				}),
			deleteBtn,
		)
}

func (l *Library) renderList() app.UI {
	if l.loading {
		return app.Div().Style("font-size", "13px").Style("color", "#999").Text("Loading...")
	}
	if l.fetchErr != "" {
		return app.Div().Style("font-size", "13px").Style("color", "#c00").Text(fmt.Sprintf("Error: %s", l.fetchErr))
	}
	if len(l.widgetTypes) == 0 {
		return app.Div().Style("font-size", "13px").Style("color", "#999").Text("No widget types found.")
	}

	items := make([]app.UI, len(l.widgetTypes))
	for i, it := range l.widgetTypes {
		id := it.ID
		name := it.Name
		var item app.UI
		if l.editingID == id {
			item = app.Div().
				Style("padding", "2px 4px").
				Style("border-radius", "4px").
				Body(
					app.Input().
						Type("text").
						Value(l.editingName).
						AutoFocus(true).
						Style("width", "100%").
						Style("font-size", "13px").
						Style("padding", "3px 4px").
						Style("border", "1px solid #0066cc").
						Style("border-radius", "2px").
						Style("box-sizing", "border-box").
						OnInput(func(ctx app.Context, e app.Event) {
							l.editingName = ctx.JSSrc().Get("value").String()
						}).
						OnBlur(func(ctx app.Context, e app.Event) {
							l.commitEdit(ctx)
						}).
						OnKeyDown(func(ctx app.Context, e app.Event) {
							switch e.Get("key").String() {
							case "Enter":
								l.commitEdit(ctx)
							case "Escape":
								l.editingID = ""
								l.editingName = ""
							}
						}),
				)
		} else {
			item = app.Div().
				Style("padding", "6px 8px").
				Style("cursor", "pointer").
				Style("border-radius", "4px").
				Style("font-size", "13px").
				Body(app.Text(name)).
				OnClick(func(ctx app.Context, e app.Event) {
					l.selectItem(id)
				}).
				OnDblClick(func(ctx app.Context, e app.Event) {
					l.startEditing(id, name)
				})
			if l.selectedID == id {
				item = item.(app.HTMLDiv).
					Style("background", "#0066cc").
					Style("color", "#fff")
			} else {
				item = item.(app.HTMLDiv).Style("color", "#333")
			}
		}
		items[i] = item
	}
	return app.Div().Body(items...)
}

func (l *Library) renderEditorColumn(title, value string, onInput func(app.Context, app.Event), showApply bool) app.UI {
	var textarea app.UI
	if onInput != nil {
		textarea = app.Textarea().
			Style("flex", "1").
			Style("resize", "none").
			Style("font-family", "monospace").
			Style("font-size", "13px").
			Text(value).
			OnInput(onInput)
	} else {
		textarea = app.Textarea().
			Style("flex", "1").
			Style("resize", "none").
			Style("font-family", "monospace").
			Style("font-size", "13px").
			Disabled(true)
	}

	applyDisabled := l.selectedID == ""
	applyBtn := app.Button().
		Style("margin-top", "4px").
		Style("padding", "4px 12px").
		Style("font-size", "13px").
		Style("border", "1px solid #ccc").
		Style("border-radius", "4px").
		Style("align-self", "flex-end").
		Text("Apply").
		OnClick(func(ctx app.Context, e app.Event) {
			l.applyChanges(ctx)
		})
	if applyDisabled {
		applyBtn = applyBtn.
			Style("opacity", "0.4").
			Style("cursor", "default").
			Disabled(true)
	} else {
		applyBtn = applyBtn.Style("cursor", "pointer")
	}

	colBody := []app.UI{
		app.H3().Style("margin", "0 0 8px 0").Text(title),
		app.Div().
			Style("flex", "1").
			Style("min-height", "0").
			Style("display", "flex").
			Style("flex-direction", "column").
			Body(textarea),
	}
	if showApply {
		colBody = append(colBody, applyBtn)
	}

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Body(colBody...)
}

func (l *Library) renderPreviewColumn() app.UI {
	var content app.UI
	if l.editedHTML == "" {
		content = app.Div().
			Style("color", "#999").
			Style("font-size", "13px").
			Text("No HTML to preview.")
	} else {
		// app.Raw renders arbitrary HTML content, supporting any valid HTML including SVG
		content = app.Raw(l.editedHTML)
	}
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Body(
			app.H3().Style("margin", "0 0 8px 0").Text("Preview"),
			app.Div().
				Style("flex", "1").
				Style("min-height", "0").
				Style("overflow", "auto").
				Style("border", "1px solid #ddd").
				Body(content),
		)
}
