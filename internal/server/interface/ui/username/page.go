// Package username is the Misc section's User name page (task 90): the name a
// Save signs a saved state with. Until Users and authentication exist
// (tasks 49/50) it is a declaration kept in the browser's LocalStorage, not an
// account: it blocks nobody and proves nothing. The page is the one place the
// UI shows the name, and a Save without one is refused: the history screen says
// where to set it, and its Save stays disabled until then (task 91).
package username

import (
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ActionChanged tells the modes that the stored name moved: the next Save signs
// with it, and an open history screen offers one (task 90).
const ActionChanged = "username.changed"

// Page is the full-page editor of the user name. The Misc section owns whether
// it is shown and wires OnBack; the page keeps the field and its refusal.
type Page struct {
	app.Compo

	// OnBack asks the section to show its contents.
	OnBack func()

	compoCtx app.Context
	mounted  bool
	loaded   bool

	name  string
	error string
}

// New creates the page; Root holds it for the tab's lifetime.
func New() *Page { return &Page{} }

// Watch makes target follow the stored name: a mode registers it in OnMount, so
// a Save signs with the name as it is now, not the one the mode mounted with.
func Watch(ctx app.Context, target *string) {
	ctx.Handle(ActionChanged, func(ctx app.Context, a app.Action) {
		if name, ok := a.Value.(string); ok {
			*target = name
		}
		ctx.Update()
	})
}

// Stored is the name a Save signs with, "" when none is set.
func Stored(ctx app.Context) string {
	var name string
	if err := ctx.LocalStorage().Get(uiutil.AuthorKey, &name); err != nil {
		return ""
	}
	return name
}

func (p *Page) OnMount(ctx app.Context) {
	p.compoCtx, p.mounted = ctx, true
	p.read(ctx)
}

func (p *Page) OnDismount() { p.mounted = false }

// Reload reads the stored name afresh: the section calls it when it opens the
// page, so a page left closed for a while shows the name as it is now. The page
// reads through its own context, never through the caller's: the caller may be
// a menu row that is gone when an answer arrives.
func (p *Page) Reload() {
	if !p.mounted {
		return
	}
	p.read(p.compoCtx)
	p.compoCtx.Update()
}

// read fills the field from LocalStorage.
func (p *Page) read(ctx app.Context) {
	p.name, p.loaded = Stored(ctx), true
}

// save writes the trimmed name and tells the modes. An empty name is refused
// here: a saved state needs an author, and an empty declaration saves nothing.
func (p *Page) save(ctx app.Context) {
	name := strings.TrimSpace(p.name)
	if name == "" {
		p.error = "A user name is required."
		ctx.Update()
		return
	}
	p.name, p.error = name, ""
	ctx.LocalStorage().Set(uiutil.AuthorKey, name)
	ctx.NewActionWithValue(ActionChanged, name)
	ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
		Title:  "User name saved",
		Detail: name,
	}})
	ctx.Update()
}

// Render draws the page: the way back to the Misc section and the title, then
// the field, its refusal and Save.
func (p *Page) Render() app.UI {
	value := p.name
	if !p.loaded {
		value = ""
	}
	return app.Div().
		Attr("data-username-page", "true").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-height", "0").
		Style("font-size", "13px").
		Body(
			uiutil.PageBar("User name", "Back to the Misc section",
				func(ctx app.Context, e app.Event) {
					if p.OnBack != nil {
						p.OnBack()
					}
				}),
			app.Div().
				Style("max-width", "560px").
				Style("padding", "12px 10px").
				Body(
					app.P().
						Attr("data-username-hint", "true").
						Style("margin", "0 0 12px").
						Style("color", "var(--text-muted)").
						Text("Saved states are signed with this name. It is not an account yet: "+
							"authentication will replace it, and until then it blocks nobody."),
					app.Div().
						Style("display", "flex").
						Style("align-items", "center").
						Style("gap", "6px").
						Body(
							app.Input().
								Attr("data-username-input", "true").
								Type("text").
								Value(value).
								Placeholder("Your name").
								AutoFocus(true).
								Style("flex", "1").
								Style("padding", "4px 6px").
								Style("border", "1px solid var(--border-input)").
								Style("border-radius", "4px").
								Style("background", "var(--input-bg)").
								Style("color", "var(--text)").
								OnInput(func(ctx app.Context, _ app.Event) {
									p.name = ctx.JSSrc().Get("value").String()
								}).
								OnKeyDown(func(ctx app.Context, e app.Event) {
									if e.Get("key").String() == "Enter" {
										p.save(ctx)
									}
								}),
							uiutil.Button("Save", "accent", true, "Save the user name", uiutil.ButtonPage,
								func(ctx app.Context, e app.Event) { p.save(ctx) }).
								Attr("data-username-save", "true"),
						),
					func() app.UI {
						if p.error == "" {
							return app.Div()
						}
						return app.Div().
							Attr("data-username-error", "true").
							Style("color", "var(--error)").
							Style("font-size", "12px").
							Style("margin-top", "6px").
							Text(p.error)
					}(),
				),
		)
}
