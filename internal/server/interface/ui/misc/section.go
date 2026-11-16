// Package misc is the Misc section of the PWA (task 90): the pages that belong
// to no mode of their own — the Labels of the Organization and the user name
// the Save dialogs prefill — under one entry of the main menu. The section
// shows its contents when it is opened and a page when one is chosen; each page
// carries the way back to the contents.
package misc

import (
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/labels"
	"github.com/kipitix/growscada/internal/server/interface/ui/username"
)

// Page is a screen of the Misc section: its contents, or one of its pages.
type Page string

const (
	// PageContents lists the section's pages; it is what a page returns to.
	PageContents Page = ""
	PageLabels   Page = "labels"
	PageUserName Page = "user"
)

// Section is the Misc mode. Root owns which page is shown and wire OnPage;
// the section keeps the two pages for the tab's lifetime.
type Section struct {
	app.Compo

	// OnPage tells Root that a page of the section was opened or that the
	// section's contents were asked for, so the navigation line and the stored
	// page follow.
	OnPage func(Page)

	labels   *labels.Page
	username *username.Page

	page     Page
	shown    bool
	compoCtx app.Context
	mounted  bool
}

// New creates the section and its pages.
func New(client *apiclient.Client) *Section {
	s := &Section{labels: labels.New(client), username: username.New()}
	s.labels.OnBack = s.showContents
	s.username.OnBack = s.showContents
	return s
}

func (s *Section) OnMount(ctx app.Context) {
	s.compoCtx, s.mounted = ctx, true
}

func (s *Section) OnDismount() { s.mounted = false }

// SetShown tells the section whether it is the mode shown; Root sets it before
// rendering. A page shown alone takes the keyboard; a hidden section does not.
func (s *Section) SetShown(shown bool) { s.shown = shown }

// SetPage restores the page a previous session left, before the section mounts.
func (s *Section) SetPage(page Page) { s.page = page }

// Show shows page, reloading what it reads: Root calls it when the menu chooses
// a page or a mode asks for one.
func (s *Section) Show(ctx app.Context, page Page) {
	s.page = page
	switch page {
	case PageLabels:
		s.labels.Reload(ctx)
	case PageUserName:
		s.username.Reload(ctx)
	}
	ctx.Update()
}

// showContents returns to the section's contents.
func (s *Section) showContents() {
	s.page = PageContents
	if s.OnPage != nil {
		s.OnPage(PageContents)
	}
	if s.mounted {
		s.compoCtx.Update()
	}
}

// show opens page from the section's contents.
func (s *Section) show(page Page) {
	s.page = page
	switch page {
	case PageLabels:
		s.labels.Reload(s.compoCtx)
	case PageUserName:
		s.username.Reload(s.compoCtx)
	}
	if s.OnPage != nil {
		s.OnPage(page)
	}
	if s.mounted {
		s.compoCtx.Update()
	}
}

// Render draws the page shown: the contents, the Labels page or the User name
// page.
func (s *Section) Render() app.UI {
	s.labels.Shown = s.shown && s.page == PageLabels
	switch s.page {
	case PageLabels:
		return s.labels
	case PageUserName:
		return s.username
	default:
		return s.renderContents()
	}
}

// renderContents lists the section's pages, each with the line that says what
// it is for.
func (s *Section) renderContents() app.UI {
	return app.Div().
		Attr("data-misc-page", "contents").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-height", "0").
		Style("font-size", "13px").
		Body(
			app.Div().
				Style("padding", "6px 10px").
				Style("border-bottom", "1px solid var(--border)").
				Body(app.Span().
					Attr("data-misc-title", "true").
					Style("font-size", "13px").
					Style("font-weight", "600").
					Text("Misc")),
			app.Div().
				Style("padding", "4px 10px").
				Body(
					s.renderEntry(PageLabels, "Labels",
						"The names and colors of the marks on saved states."),
					s.renderEntry(PageUserName, "User name",
						"The name the Save dialogs sign a saved state with."),
				),
		)
}

// renderEntry is one page of the section: its name and what it is for.
func (s *Section) renderEntry(page Page, name, description string) app.UI {
	return app.Button().
		Attr("data-misc-entry", string(page)).
		Attr("role", "menuitem").
		Title(description).
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("align-items", "flex-start").
		Style("gap", "2px").
		Style("width", "100%").
		Style("padding", "8px 10px").
		Style("border", "none").
		Style("border-bottom", "1px solid var(--border-subtle)").
		Style("background", "transparent").
		Style("color", "var(--text)").
		Style("text-align", "left").
		Style("cursor", "pointer").
		Body(
			app.Span().
				Style("font-size", "14px").
				Style("font-weight", "600").
				Text(name),
			app.Span().
				Style("font-size", "12px").
				Style("color", "var(--text-muted)").
				Text(description),
		).
		OnClick(func(ctx app.Context, e app.Event) { s.show(page) })
}
