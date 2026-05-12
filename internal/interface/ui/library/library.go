package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

type indicatorTypeItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	SvgTemplate    string `json:"svg_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
}

type getIndicatorTypesResponse struct {
	IndicatorTypes []indicatorTypeItem `json:"indicator_types"`
}

type createIndicatorTypeRequest struct {
	Name           string `json:"name"`
	SvgTemplate    string `json:"svg_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
}

type createIndicatorTypeResponse struct {
	ID string `json:"id"`
}

type updateIndicatorTypeRequest struct {
	Name           string `json:"name"`
	SvgTemplate    string `json:"svg_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
}

type Library struct {
	app.Compo
	apiServerURL   string
	indicatorTypes []indicatorTypeItem
	loading        bool
	fetchErr       string
	selectedID          string
	editedName          string
	editedSVG           string
	editedScript        string
	editedScriptLang    string
	newItemName         string
}

func NewLibrary(apiServerURL string) *Library {
	return &Library{apiServerURL: apiServerURL}
}

func (l *Library) OnMount(ctx app.Context) {
	l.loadList(ctx)
}

func (l *Library) loadList(ctx app.Context) {
	l.loading = true
	url := l.apiServerURL + "/api/v1/indicator-types"
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

		var result getIndicatorTypesResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				l.loading = false
				l.fetchErr = err.Error()
			})
			return
		}

		ctx.Dispatch(func(ctx app.Context) {
			l.loading = false
			l.indicatorTypes = result.IndicatorTypes
		})
	})
}

const defaultSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"></svg>`
const defaultScript = "function update() {\n}"

func (l *Library) createItem(ctx app.Context) {
	name := l.newItemName
	if name == "" {
		name = "New Indicator Type"
	}
	url := l.apiServerURL + "/api/v1/indicator-types"
	body, _ := json.Marshal(createIndicatorTypeRequest{
		Name:           name,
		SvgTemplate:    defaultSVG,
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

		var result createIndicatorTypeResponse
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
			l.editedSVG = defaultSVG
			l.editedScript = defaultScript
			l.editedScriptLang = "javascript"
			l.newItemName = ""
			l.loadList(ctx)
		})
	})
}

func (l *Library) deleteItem(ctx app.Context) {
	if l.selectedID == "" {
		return
	}
	url := l.apiServerURL + "/api/v1/indicator-types/" + l.selectedID
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
				l.editedSVG = ""
				l.editedScript = ""
			}
			l.loadList(ctx)
		})
	})
}

func (l *Library) selectItem(id string) {
	for _, it := range l.indicatorTypes {
		if it.ID == id {
			l.selectedID = id
			l.editedName = it.Name
			l.editedSVG = it.SvgTemplate
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
	url := l.apiServerURL + "/api/v1/indicator-types/" + l.selectedID
	body, _ := json.Marshal(updateIndicatorTypeRequest{
		Name:           l.editedName,
		SvgTemplate:    l.editedSVG,
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
			l.renderEditorColumn("SVG Template", l.editedSVG, func(ctx app.Context, e app.Event) {
				l.editedSVG = ctx.JSSrc().Get("value").String()
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
			app.H3().Style("margin", "0 0 8px 0").Text("Indicator Types"),
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
		Style("flex-direction", "column").
		Style("gap", "4px").
		Style("margin-bottom", "4px").
		Body(
			app.Input().
				Type("text").
				Placeholder("Name").
				Value(l.newItemName).
				Style("width", "100%").
				Style("padding", "4px 6px").
				Style("font-size", "13px").
				Style("border", "1px solid #ccc").
				Style("border-radius", "4px").
				Style("box-sizing", "border-box").
				OnInput(func(ctx app.Context, e app.Event) {
					l.newItemName = ctx.JSSrc().Get("value").String()
				}),
			app.Div().
				Style("display", "flex").
				Style("gap", "4px").
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
				),
		)
}

func (l *Library) renderList() app.UI {
	if l.loading {
		return app.Div().Style("font-size", "13px").Style("color", "#999").Text("Loading...")
	}
	if l.fetchErr != "" {
		return app.Div().Style("font-size", "13px").Style("color", "#c00").Text(fmt.Sprintf("Error: %s", l.fetchErr))
	}
	if len(l.indicatorTypes) == 0 {
		return app.Div().Style("font-size", "13px").Style("color", "#999").Text("No indicator types found.")
	}

	items := make([]app.UI, len(l.indicatorTypes))
	for i, it := range l.indicatorTypes {
		id := it.ID
		name := it.Name
		item := app.Div().
			Style("padding", "6px 8px").
			Style("cursor", "pointer").
			Style("border-radius", "4px").
			Style("font-size", "13px").
			Body(app.Text(name)).
			OnClick(func(ctx app.Context, e app.Event) {
				l.selectItem(id)
			})
		if l.selectedID == id {
			item = item.
				Style("background", "#0066cc").
				Style("color", "#fff")
		} else {
			item = item.Style("color", "#333")
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
	if l.editedSVG == "" {
		content = app.Div().
			Style("color", "#999").
			Style("font-size", "13px").
			Text("No SVG to preview.")
	} else {
		content = app.Raw(l.editedSVG)
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
