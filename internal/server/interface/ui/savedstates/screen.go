// Package savedstates is the screen that lists a Draft's saved states, shared
// by the Library and the Project modes (task 84). Both modes keep their own
// data and commands — a library's and a project's saved states are read and
// written by their own services —, so this package draws the screen and calls
// back: it knows nothing of the API, of a Draft or of a Deploy beyond the
// numbers and words a row carries. The labels of a saved state are drawn here
// alone: colored chips and a menu of the Organization's labels (task 85_3;
// ADR 0025). A saved state is loaded into the Draft from here; publishing and
// deploying it belong to the mode's own windows (task 85_7).
package savedstates

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// Row is one saved state as the screen shows it: what the history read and
// the labels that mark it.
type Row struct {
	Number  int
	Title   string
	Author  string
	SavedAt time.Time
	// Labels are the marks of the saved state, ordered by name ignoring case
	// (task 85_3).
	Labels []apiv0.LabelResponse
}

// Screen draws the saved states of one Draft, the Draft itself as its top row.
// The mode sets its fields before rendering it and keeps the commands: the
// screen calls them with the row they act on. The way back belongs to the
// panel, not to the screen (task 91).
type Screen struct {
	app.Compo

	// Rows is the history, newest first; Loaded is false until its first
	// answer, and Loading marks a load of a saved state in flight, which
	// disables every Load button.
	Rows    []Row
	Loaded  bool
	Loading bool

	// AllLabels are the labels of the Organization, the menu of every row:
	// a saved state may carry any number of them (task 85_3).
	AllLabels []apiv0.LabelResponse

	// CanSave reports whether the Draft has changes worth saving and Saving
	// marks a Save in flight; the form at the top offers Save only then
	// (task 91). OnSave fixes the Draft as a new saved state under the name
	// the form holds.
	CanSave bool
	Saving  bool
	OnSave  func(ctx app.Context, title string)

	// OnLoad and OnRename act on the row they name. OnAssign places a label
	// on the row's saved state and OnUnassign takes it off; OnManageLabels
	// opens the Labels page, where new labels are named and colored. A mode
	// confirms a load on its own: the screen only says which row was asked
	// for.
	OnLoad         func(ctx app.Context, number int)
	OnRename       func(ctx app.Context, number int, title string)
	OnAssign       func(ctx app.Context, number int, labelID uuid.UUID)
	OnUnassign     func(ctx app.Context, number int, labelID uuid.UUID)
	OnManageLabels func(ctx app.Context)

	// newTitle is the name the form at the top gives the next saved state.
	// StartNew fills it when the screen opens; typing keeps it here until the
	// Save or the screen closes.
	newTitle string

	// renaming is the number of the saved state whose name is edited in place
	// (0: none), renameText the text of the editor. The screen owns them: a
	// name is the screen's own state until it is committed. labelMenu is the
	// number of the saved state whose labels menu is open (0: none).
	renaming   int
	renameText string
	labelMenu  int
}

// New creates the screen.
func New() *Screen { return &Screen{} }

// Reset drops what the screen kept for itself — the name being edited, the
// name the form holds and the open labels menu: the screen closes, and the
// next one opens on the names and labels as they are.
func (s *Screen) Reset() { s.renaming, s.renameText, s.labelMenu, s.newTitle = 0, "", 0, "" }

// StartNew sets the name the form at the top starts from; the mode calls it
// when it opens the screen (task 91).
func (s *Screen) StartNew(title string) { s.newTitle = title }

// canSave reports whether the form may fix the Draft as a saved state: the
// mode says the Draft has changes, no Save is in flight and a name is given.
func (s *Screen) canSave() bool {
	return s.CanSave && !s.Saving && strings.TrimSpace(s.newTitle) != ""
}

// save hands the name to the mode, which sends the Save: the screen keeps the
// text until the answer, so a refusal leaves it in place.
func (s *Screen) save(ctx app.Context) {
	if !s.canSave() || s.OnSave == nil {
		return
	}
	s.OnSave(ctx, strings.TrimSpace(s.newTitle))
}

// Render draws the screen: the title, the Draft as its newest state at the top
// of the list — with the form that names a new saved state —, then the saved
// states (task 91).
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
				Attr("data-history-bar", "true").
				Style("display", "flex").
				Style("flex-wrap", "wrap").
				Style("align-items", "center").
				Style("gap", "8px").
				Style("padding", "6px 10px").
				Style("border-bottom", "1px solid var(--border)").
				Body(
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
				Body(append([]app.UI{s.renderDraftRow()}, rows...)...),
		)
}

// renderDraftRow is the Draft itself as the newest state: always the top row,
// with the name the next Save gives it. Only the Save is enabled when the
// Draft has changes (task 91).
func (s *Screen) renderDraftRow() app.UI {
	saveTitle := "Save the Draft as a saved state"
	switch {
	case s.Saving:
		saveTitle = "The Draft is being saved"
	case !s.CanSave:
		saveTitle = "Nothing to save since the last save"
	}
	return app.Div().
		Attr("data-draft-state-row", "true").
		Style("display", "flex").
		Style("flex-wrap", "wrap").
		Style("align-items", "center").
		Style("gap", "10px").
		Style("padding", "6px 4px").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Span().
				Attr("data-draft-state-label", "true").
				Style("font-size", "11px").
				Style("font-weight", "600").
				Style("width", "118px").
				Style("flex-shrink", "0").
				Text("Draft"),
			app.Input().
				Attr("data-save-title", "true").
				Type("text").
				Value(s.newTitle).
				Placeholder("Name of the saved state").
				Style("flex", "1").
				Style("min-width", "160px").
				Style("box-sizing", "border-box").
				Style("padding", "3px 6px").
				Style("font-size", "12px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, e app.Event) {
					s.newTitle = ctx.JSSrc().Get("value").String()
					ctx.Update()
				}).
				OnKeyDown(func(ctx app.Context, e app.Event) {
					if e.Get("key").String() == "Enter" {
						s.save(ctx)
					}
				}),
			uiutil.IconButton(uiutil.IconSave(), "accent", s.canSave(), saveTitle,
				uiutil.ButtonRow, func(ctx app.Context, e app.Event) { s.save(ctx) }).
				Attr("data-save-confirm", "true"),
		)
}

// renderRow is one saved state: the time it was saved, its name and labels —
// edited in place —, its author, and the action that ends it: load it into the
// Draft (task 85_7).
func (s *Screen) renderRow(row Row) app.UI {
	number := strconv.Itoa(row.Number)
	details := []app.UI{s.renderName(row)}
	details = append(details, s.renderLabels(row))

	load := uiutil.IconButton(uiutil.IconLoad(), "", !s.Loading,
		"Replace the Draft with this saved state",
		uiutil.ButtonRow, func(ctx app.Context, e app.Event) {
			if s.OnLoad != nil {
				s.OnLoad(ctx, row.Number)
			}
		}, app.EventScope("load/"+number))
	actions := []app.UI{load}

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
			app.Span().Style("display", "flex").Style("gap", "4px").Body(actions...),
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

// renderLabels is the saved state's marks: one colored chip per label and the
// tag that opens the menu of them. The control opens the menu of the Organization's labels,
// each marked when the saved state carries it; clicking one places it or takes
// it off. A menu with no label says so and offers the Labels page instead.
func (s *Screen) renderLabels(row Row) app.UI {
	number := strconv.Itoa(row.Number)
	body := []app.UI{}
	assigned := make(map[uuid.UUID]bool, len(row.Labels))
	chips := app.Div().Style("display", "flex").Style("flex-wrap", "wrap").Style("align-items", "center").Style("gap", "4px")
	chipsBody := []app.UI{}
	for _, l := range row.Labels {
		assigned[l.ID] = true
		chipsBody = append(chipsBody, LabelChip(l))
	}
	chipsBody = append(chipsBody, uiutil.IconButton(uiutil.IconLabel(), "", true, "Label this saved state", uiutil.ButtonRow,
		func(ctx app.Context, e app.Event) {
			if s.labelMenu == row.Number {
				s.labelMenu = 0
			} else {
				s.labelMenu = row.Number
			}
			ctx.Update()
		}, app.EventScope("label-menu/"+number)).Attr("data-label-add", number))
	body = append(body, chips.Body(chipsBody...))
	if s.labelMenu == row.Number {
		body = append(body, s.renderLabelMenu(row, assigned))
	}
	return app.Div().
		Attr("data-saved-state-labels", number).
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("gap", "2px").
		Style("margin-top", "2px").
		Body(body...)
}

// renderLabelMenu is the labels of the Organization as a menu: a tick marks the
// one the saved state carries, and a click places or takes it off. An empty
// list sends the person to the Labels page.
func (s *Screen) renderLabelMenu(row Row, assigned map[uuid.UUID]bool) app.UI {
	number := strconv.Itoa(row.Number)
	if len(s.AllLabels) == 0 {
		return app.Div().
			Attr("data-label-menu", number).
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "6px").
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Body(
				app.Span().Text("No labels yet"),
				uiutil.IconButton(uiutil.IconLabel(), "", true, "Open the Labels page", uiutil.ButtonRow,
					func(ctx app.Context, e app.Event) {
						if s.OnManageLabels != nil {
							s.OnManageLabels(ctx)
						}
					}, app.EventScope("manage-labels/"+number)),
			)
	}
	items := make([]app.UI, 0, len(s.AllLabels))
	for _, l := range s.AllLabels {
		labelID := l.ID
		chosen := assigned[labelID]
		mark := " "
		if chosen {
			mark = "✓"
		}
		button := uiutil.Button(mark+" "+l.Name, "", true,
			labelTitle(l, chosen), uiutil.ButtonRow, func(ctx app.Context, e app.Event) {
				if chosen {
					if s.OnUnassign != nil {
						s.OnUnassign(ctx, row.Number, labelID)
					}
				} else if s.OnAssign != nil {
					s.OnAssign(ctx, row.Number, labelID)
				}
				s.labelMenu = 0
				ctx.Update()
			}, app.EventScope("label-option/"+number+"/"+labelID.String())).
			Attr("data-label-option", labelID.String())
		if chosen {
			button = button.Attr("data-label-checked", "true")
		}
		items = append(items, button)
	}
	return app.Div().
		Attr("data-label-menu", number).
		Style("display", "flex").
		Style("flex-wrap", "wrap").
		Style("gap", "4px").
		Body(items...)
}

// LabelChip is one label of a saved state: a small colored chip that always
// shows the name, so the color is never the only difference. The history and
// the Deploy window draw the labels of a saved state with it (task 85_3,
// task 85_5).
func LabelChip(l apiv0.LabelResponse) app.UI {
	border, text := "var(--border)", "var(--text-muted)"
	if l.Color != "" {
		border, text = l.Color, "var(--text)"
	}
	return app.Span().
		Attr("data-label-id", l.ID.String()).
		Attr("data-label-chip", l.Color).
		Style("padding", "0 6px").
		Style("font-size", "11px").
		Style("border", "1px solid "+border).
		Style("border-radius", "9px").
		Style("color", text).
		Text(l.Name)
}

// labelTitle is the hover text of one label of the menu.
func labelTitle(l apiv0.LabelResponse, chosen bool) string {
	if chosen {
		return "Take " + l.Name + " off this saved state"
	}
	return "Mark this saved state " + l.Name
}
