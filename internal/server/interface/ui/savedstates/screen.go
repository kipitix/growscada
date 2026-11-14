// Package savedstates is the screen that lists a Draft's saved states, shared
// by the Library and the Project modes (task 84). Both modes keep their own
// data and commands — a library's and a project's saved states are read and
// written by their own services —, so this package draws the screen and calls
// back: it knows nothing of the API, of a Draft or of a Release beyond the
// numbers and words a row carries.
package savedstates

import (
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// Row is one saved state as the screen shows it: what the history read and,
// when a Release published it, that Release's number and note.
type Row struct {
	Number  int
	Title   string
	Author  string
	SavedAt time.Time
	// Released says a Release published this saved state; Release is its
	// number (0 for a Release made before saved states existed) and Note its
	// note.
	Released bool
	Release  int
	Note     string
}

// Screen draws the saved states of one Draft and the way back to it. The mode
// sets its fields before rendering it and keeps the commands: the screen calls
// them with the row they act on.
type Screen struct {
	app.Compo

	// Rows is the history, newest first; Loaded is false until its first
	// answer, and Loading marks a load of a saved state in flight, which
	// disables every Load button.
	Rows    []Row
	Loaded  bool
	Loading bool

	// OnBack shows the Draft again; OnLoad, OnRelease, OnRename and OnNote
	// act on the row they name. A mode confirms a load and asks for a note on
	// its own: the screen only says which row was asked for.
	OnBack    func(ctx app.Context)
	OnLoad    func(ctx app.Context, number int)
	OnRelease func(ctx app.Context, number int)
	OnRename  func(ctx app.Context, number int, title string)
	OnNote    func(ctx app.Context, number int)

	// renaming is the number of the saved state whose name is edited in place
	// (0: none), renameText the text of the editor. The screen owns them: a
	// name is the screen's own state until it is committed.
	renaming   int
	renameText string
}

// New creates the screen.
func New() *Screen { return &Screen{} }

// ResetRename drops the name being edited: the screen closes, and the next one
// opens on the names as they are.
func (s *Screen) ResetRename() { s.renaming, s.renameText = 0, "" }

// Render draws the screen: the way back with its title, then the rows in a list
// that scrolls in its own pane.
func (s *Screen) Render() app.UI {
	rows := []app.UI{}
	switch {
	case !s.Loaded:
		rows = append(rows, app.Div().
			Style("padding", "8px").
			Style("color", "var(--text-muted)").
			Text("Loading saved states…"))
	case len(s.Rows) == 0:
		rows = append(rows, app.Div().
			Style("padding", "8px").
			Text("Nothing has been saved yet."))
	default:
		for _, row := range s.Rows {
			rows = append(rows, s.renderRow(row))
		}
	}
	return app.Div().
		Attr("data-history-screen", "true").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-height", "0").
		Body(
			app.Div().
				Style("display", "flex").
				Style("align-items", "center").
				Style("gap", "8px").
				Style("padding", "6px 10px").
				Style("border-bottom", "1px solid var(--border)").
				Body(
					uiutil.PanelButton(uiutil.IconBack(), true, "Back to the Draft",
						func(ctx app.Context, e app.Event) {
							if s.OnBack != nil {
								s.OnBack(ctx)
							}
						}, app.EventScope("history-back")),
					app.Span().
						Attr("data-history-title", "true").
						Style("font-size", "13px").
						Style("font-weight", "600").
						Text("Saved states"),
				),
			app.Div().
				Attr("data-history-list", "true").
				Style("flex", "1").
				Style("min-height", "0").
				Style("overflow-y", "auto").
				Style("padding", "4px 10px").
				Body(rows...),
		)
}

// renderRow is one saved state: the time it was saved, its name — edited in
// place —, the Release that published it, its author, and the two actions that
// end it: load it into the Draft, or publish it as the next Release.
func (s *Screen) renderRow(row Row) app.UI {
	number := strconv.Itoa(row.Number)
	details := []app.UI{s.renderName(row)}
	if row.Released {
		details = append(details, s.renderRelease(row))
	}

	load := uiutil.Button("Load", "", !s.Loading,
		"Replace the Draft with this saved state",
		uiutil.ButtonRow, func(ctx app.Context, e app.Event) {
			if s.OnLoad != nil {
				s.OnLoad(ctx, row.Number)
			}
		}, app.EventScope("load/"+number))
	release := uiutil.Button("Make Release", "accent", !row.Released,
		"Publish this saved state as the next Release",
		uiutil.ButtonRow, func(ctx app.Context, e app.Event) {
			if s.OnRelease != nil {
				s.OnRelease(ctx, row.Number)
			}
		}, app.EventScope("release/"+number))
	if row.Released {
		release = uiutil.Button("Released", "", false, "A Release publishes this saved state",
			uiutil.ButtonRow, func(app.Context, app.Event) {}, app.EventScope("released/"+number))
	}

	return app.Div().
		Attr("data-saved-state-row", number).
		Style("display", "flex").
		Style("flex-wrap", "wrap").
		Style("align-items", "center").
		Style("gap", "10px").
		Style("padding", "6px 4px").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Span().
				Attr("data-saved-state-time", number).
				Style("font-size", "11px").
				Style("color", "var(--text-muted)").
				Style("width", "118px").
				Style("flex-shrink", "0").
				Text(uiutil.FormatTime(row.SavedAt)),
			app.Div().Style("flex", "1").Style("min-width", "0").Body(details...),
			app.Span().
				Attr("data-saved-state-author", number).
				Style("font-size", "11px").
				Style("color", "var(--text-muted)").
				Text(row.Author),
			load,
			release,
		)
}

// renderName is the saved state's name, or its editor: a field with a tick that
// saves and a cross that drops the change.
func (s *Screen) renderName(row Row) app.UI {
	number := strconv.Itoa(row.Number)
	if s.renaming == row.Number {
		return app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "4px").
			Body(
				app.Input().
					Attr("data-saved-state-rename", number).
					Type("text").
					Value(s.renameText).
					AutoFocus(true).
					Style("font-size", "12px").
					Style("padding", "2px 4px").
					OnInput(func(ctx app.Context, e app.Event) {
						s.renameText = ctx.JSSrc().Get("value").String()
					}),
				uiutil.PanelButton(uiutil.IconCheck(), true, "Save the name",
					func(ctx app.Context, e app.Event) {
						title := strings.TrimSpace(s.renameText)
						if title == "" {
							return
						}
						if s.OnRename != nil {
							s.OnRename(ctx, row.Number, title)
						}
						s.renaming, s.renameText = 0, ""
					}, app.EventScope("rename-ok/"+number)),
				uiutil.PanelButton(uiutil.IconClose(), true, "Drop the new name",
					func(ctx app.Context, e app.Event) {
						s.renaming, s.renameText = 0, ""
						ctx.Update()
					}, app.EventScope("rename-cancel/"+number)),
			)
	}
	return app.Div().
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "4px").
		Body(
			app.Span().
				Attr("data-saved-state-title", number).
				Style("font-size", "13px").
				Text(row.Title),
			uiutil.PanelButton(uiutil.IconPencil(), true, "Rename the saved state",
				func(ctx app.Context, e app.Event) {
					s.renaming, s.renameText = row.Number, row.Title
					ctx.Update()
				}, app.EventScope("rename/"+number)),
		)
}

// renderRelease is the Release that published the saved state: its number and
// note, the note edited in place. A Release made before saved states existed
// has no number of them and is named as such.
func (s *Screen) renderRelease(row Row) app.UI {
	number := strconv.Itoa(row.Number)
	label := "Release " + strconv.Itoa(row.Release)
	if row.Release == 0 {
		label = "Legacy Release"
	}
	if row.Note != "" {
		label += " · " + row.Note
	}
	return app.Div().
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "4px").
		Body(
			app.Span().
				Attr("data-saved-state-release", number).
				Style("font-size", "11px").
				Style("color", "var(--accent)").
				Text(label),
			uiutil.PanelButton(uiutil.IconPencil(), true, "Edit the Release's note",
				func(ctx app.Context, e app.Event) {
					if s.OnNote != nil {
						s.OnNote(ctx, row.Number)
					}
				}, app.EventScope("note/"+number)),
		)
}
