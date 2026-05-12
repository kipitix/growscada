package library

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

type indicatorTypeItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type getIndicatorTypesResponse struct {
	IndicatorTypes []indicatorTypeItem `json:"indicator_types"`
}

type Library struct {
	app.Compo
	apiServerURL   string
	indicatorTypes []indicatorTypeItem
	loading        bool
	fetchErr       string
	selectedID     string
}

func NewLibrary(apiServerURL string) *Library {
	return &Library{apiServerURL: apiServerURL}
}

func (l *Library) OnMount(ctx app.Context) {
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

func (l *Library) Render() app.UI {
	return app.Div().Class("library").Body(
		app.Div().Class("library-sidebar").Body(
			app.H2().Text("Indicator Types"),
			l.renderList(),
		),
		app.Div().Class("library-content").Body(
			app.H2().Text("Editor"),
		),
	)
}

func (l *Library) renderList() app.UI {
	if l.loading {
		return app.Div().Class("library-list-status").Text("Loading...")
	}
	if l.fetchErr != "" {
		return app.Div().Class("library-list-status library-list-error").Text(fmt.Sprintf("Error: %s", l.fetchErr))
	}
	if len(l.indicatorTypes) == 0 {
		return app.Div().Class("library-list-status").Text("No indicator types found.")
	}

	items := make([]app.UI, len(l.indicatorTypes))
	for i, it := range l.indicatorTypes {
		id := it.ID
		name := it.Name
		class := "library-list-item"
		if l.selectedID == id {
			class += " library-list-item--selected"
		}
		items[i] = app.Div().
			Class(class).
			Body(app.Text(name)).
			OnClick(func(ctx app.Context, e app.Event) {
				l.selectedID = id
			})
	}
	return app.Div().Class("library-list").Body(items...)
}
