package library

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

const defaultHTML = `<div class="widget"></div>`
const defaultScript = "function update() {\n}"

// ── Widget type CRUD ──────────────────────────────────────────────────────────

func (l *Library) createItem(ctx app.Context) {
	name := "New Widget Type"
	url := l.apiServerURL + "/api/v1/widget-types"
	body, _ := json.Marshal(createWidgetTypeRequest{
		Name:           name,
		HtmlTemplate:   defaultHTML,
		Script:         defaultScript,
		ScriptLanguage: "javascript",
		DefaultWidth:   120,
		DefaultHeight:  60,
		InputPorts:     []inputPortDTO{},
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
			l.editedInputValues = make(map[string]string)
			l.editedInputPorts = nil
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
				l.editedInputValues = make(map[string]string)
				l.editedInputPorts = nil
			}
			l.loadList(ctx)
		})
	})
}

func (l *Library) applyChanges(ctx app.Context) {
	if l.selectedID == "" {
		return
	}
	var currentItem widgetTypeItem
	for _, it := range l.widgetTypes {
		if it.ID == l.selectedID {
			currentItem = it
			break
		}
	}
	url := l.apiServerURL + "/api/v1/widget-types/" + l.selectedID
	ports := l.editedInputPorts
	if ports == nil {
		ports = []inputPortDTO{}
	}
	body, _ := json.Marshal(updateWidgetTypeRequest{
		Name:           l.editedName,
		HtmlTemplate:   l.editedHTML,
		Script:         l.editedScript,
		ScriptLanguage: l.editedScriptLang,
		DefaultWidth:   currentItem.DefaultWidth,
		DefaultHeight:  currentItem.DefaultHeight,
		InputPorts:     ports,
		Version:        currentItem.Version,
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

	for i, it := range l.widgetTypes {
		if it.ID == id {
			l.widgetTypes[i].Name = name
			break
		}
	}
	if l.selectedID == id {
		l.editedName = name
	}

	url := l.apiServerURL + "/api/v1/widget-types/" + id
	foundPorts := found.InputPorts
	if foundPorts == nil {
		foundPorts = []inputPortDTO{}
	}
	body, _ := json.Marshal(updateWidgetTypeRequest{
		Name:           name,
		HtmlTemplate:   found.HtmlTemplate,
		Script:         found.Script,
		ScriptLanguage: found.ScriptLanguage,
		DefaultWidth:   found.DefaultWidth,
		DefaultHeight:  found.DefaultHeight,
		InputPorts:     foundPorts,
		Version:        found.Version,
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
