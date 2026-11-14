package project

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/savedstates"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Library Imports ───────────────────────────────────────────────────────────
//
// The project uses the WidgetTypes of the saved states of the Libraries it
// imports, one per library (ADR 0007, task 85_6). The panel lists them and the
// Import libraries window chooses one: the libraries, each with its saved
// states narrowed by a label. A switch or removal is previewed first: Widgets
// it would leave without their WidgetType block it, the PortBindings it would
// break need a confirmation, and the server removes them in the same change
// (ADR 0012).

// libraryName returns the name of the library id, or its ID while the
// libraries are not loaded.
func (p *Project) libraryName(id uuid.UUID) string {
	if l, ok := uiutil.FindByID(p.libraries, id, uiutil.LibraryID); ok {
		return l.Name
	}
	return id.String()
}

// importOf returns the project's import of the library id.
func (p *Project) importOf(id uuid.UUID) (apiv0.LibraryImportResponse, bool) {
	for _, imported := range p.imports {
		if imported.LibraryID == id {
			return imported, true
		}
	}
	return apiv0.LibraryImportResponse{}, false
}

// importProvenance describes the saved state an import names: its number, its
// title and the Author who saved it (task 85_6).
func (p *Project) importProvenance(imported apiv0.LibraryImportResponse) string {
	provenance := fmt.Sprintf("saved state #%d", imported.CommitNumber)
	if imported.Title != "" {
		provenance += " " + imported.Title
	}
	if imported.Author != "" {
		provenance += " · " + imported.Author
	}
	return provenance
}

// ── Import libraries window (task 85_6) ─────────────────────────────────────

// importListKey names the list of saved states the window shows: the library
// opened and the label the filter states. An answer of another library or
// another label is dropped and the list reruns (uiutil.LoadFor).
type importListKey struct {
	library *apiclient.LibraryClient
	label   uuid.UUID
}

// openImportDialog shows the window with every Library of the server, and the
// labels its filter offers.
func (p *Project) openImportDialog(ctx app.Context) {
	p.importDialog = true
	p.importLibraryID = uuid.Nil
	p.importCommits, p.importCommitsLoaded = nil, false
	p.loadImports(ctx)
	p.loadLibraries(ctx)
	p.loadHistoryLabels(ctx)
	ctx.Update()
}

// closeImportDialog closes the window and forgets the library it opened.
func (p *Project) closeImportDialog() {
	p.importDialog = false
	p.importLibraryID = uuid.Nil
	p.importCommits, p.importCommitsLoaded = nil, false
	p.importLabel = uuid.Nil
}

// setImportLabel narrows the saved states of the opened library to those the
// label marks; the zero UUID asks for all of them. The filter does not narrow
// the list of libraries.
func (p *Project) setImportLabel(ctx app.Context, labelID uuid.UUID) {
	if labelID == p.importLabel {
		return
	}
	p.importLabel = labelID
	p.loadImportCommits(ctx)
	ctx.Update()
}

// selectImportLibrary opens the saved states of the library, or closes them
// when it is the one opened already.
func (p *Project) selectImportLibrary(ctx app.Context, libraryID uuid.UUID) {
	if p.importLibraryID == libraryID {
		p.importLibraryID = uuid.Nil
		p.importCommits, p.importCommitsLoaded = nil, false
		ctx.Update()
		return
	}
	p.importLibraryID = libraryID
	p.importCommits, p.importCommitsLoaded = nil, false
	p.loadImportCommits(ctx)
	ctx.Update()
}

// loadImportCommits (re)loads the saved states of the opened library, newest
// first: every one, or those the chosen label marks. A library whose read
// fails — it was removed meanwhile — closes its list rather than showing
// another library's states in its place.
func (p *Project) loadImportCommits(ctx app.Context) {
	libraryID := p.importLibraryID
	if p.client == nil || libraryID == uuid.Nil {
		p.importCommits, p.importCommitsLoaded = nil, false
		return
	}
	library := p.client.Library(libraryID)
	uiutil.LoadForGone(ctx, &p.importCommitsReloader,
		func() importListKey { return importListKey{library: library, label: p.importLabel} },
		func(c context.Context, k importListKey) ([]apiv0.CommitResponse, error) {
			if k.label == uuid.Nil {
				return k.library.Commits(c)
			}
			return k.library.CommitsByLabel(c, k.label)
		},
		func(ctx app.Context, _ importListKey, commits []apiv0.CommitResponse) {
			if p.importLibraryID != libraryID {
				return
			}
			p.importCommits, p.importCommitsLoaded = commits, true
		},
		func(ctx app.Context, k importListKey) {
			// The library was removed meanwhile: its states say nothing of the
			// project, and the list of libraries is reread.
			if p.importLibraryID == libraryID {
				p.importLibraryID, p.importCommits, p.importCommitsLoaded = uuid.Nil, nil, false
			}
			p.loadLibraries(ctx)
			ctx.Update()
		}, p.loadImportCommits)
}

// changeImport imports the saved state commit of the library, or removes the
// import for commit 0, after its preview: orphaned Widgets refuse it, broken
// bindings are confirmed.
func (p *Project) changeImport(libraryID uuid.UUID, commit int) {
	if p.waitMoves(p.compoCtx, func(ctx app.Context) { p.changeImport(libraryID, commit) }) {
		return
	}
	imported, hasImport := p.importOf(libraryID)
	if p.content == nil || (hasImport && imported.CommitNumber == commit) || (!hasImport && commit == 0) {
		return
	}
	content := p.content
	uiutil.Call(p.compoCtx, func(c context.Context) (apiv0.ImportImpactResponse, error) {
		return content.ImportImpact(c, libraryID, commit)
	}, func(ctx app.Context, impact apiv0.ImportImpactResponse) {
		what := fmt.Sprintf("import saved state %d of %s", commit, p.libraryName(libraryID))
		if commit == 0 {
			what = "remove the import of " + p.libraryName(libraryID)
		}
		if len(impact.OrphanedWidgets) > 0 {
			p.showOrphaned(ctx, what, impact.OrphanedWidgets)
			return
		}
		if len(impact.BrokenBindings) > 0 {
			lines := make([]string, len(impact.BrokenBindings))
			for i, b := range impact.BrokenBindings {
				lines[i] = fmt.Sprintf("• %s: port %q", b.Name, b.PortName)
			}
			message := fmt.Sprintf("To %s, these bindings will be removed:\n%s\n\nContinue?", what, strings.Join(lines, "\n"))
			if !app.Window().Call("confirm", message).Bool() {
				return
			}
		}
		p.saveImport(content, libraryID, commit, p.draftVersion())
	}, nil)
}

// saveImport imports (commit > 0) or removes (commit 0) the import.
func (p *Project) saveImport(content *apiclient.ProjectClient, libraryID uuid.UUID, commit, version int) {
	mark := p.markDraft()
	uiutil.Call(p.compoCtx, func(c context.Context) (apiv0.LibraryImportResponse, error) {
		if commit == 0 {
			return content.RemoveImport(c, libraryID, version)
		}
		return content.ImportLibrary(c, libraryID, apiv0.PutLibraryImportRequest{Commit: commit, Version: version})
	}, func(ctx app.Context, saved apiv0.LibraryImportResponse) {
		p.observeDraftVersion(mark, saved.Version)
		p.loadImports(ctx)
		p.loadWidgetTypes(ctx)
		p.loadDraftState(ctx)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		// Widgets placed since the preview: the refusal names them.
		var se *apiclient.StatusError
		if errors.As(err, &se) && len(se.Problem.OrphanedWidgets) > 0 {
			p.showOrphaned(ctx, "change the import", se.Problem.OrphanedWidgets)
		} else {
			uiutil.ShowError(ctx, err)
		}
		if errors.Is(err, apiclient.ErrConflict) {
			p.forgetDraftVersion()
			p.loadDraftState(ctx)
			p.loadImports(ctx)
		}
		ctx.Update()
	})
}

// showOrphaned tells that the change what is refused while the widgets are of
// WidgetTypes it would drop.
func (p *Project) showOrphaned(ctx app.Context, what string, widgets []apiv0.PinnedWidgetResponse) {
	names := make([]string, len(widgets))
	for i, w := range widgets {
		scene := w.SceneID.String()
		if sc, ok := p.findScene(w.SceneID); ok {
			scene = sc.Name
		}
		names[i] = fmt.Sprintf("%s (%s)", w.Name, scene)
	}
	ctx.NewActionWithValue(toast.ActionAdd, toast.Problem{Problem: apiv0.Problem{
		Title:  "Cannot " + what,
		Detail: "Widgets would be left without their WidgetType; remove them or change their type first: " + strings.Join(names, ", "),
	}})
}

// ── The panel ───────────────────────────────────────────────────────────────

// renderImportsPanel renders the Imported libraries panel: each imported
// library with the saved state it names, a removal, and the way into the
// Import libraries window. The widget types of a library are chosen there, not
// here (task 85_6).
func (p *Project) renderImportsPanel() app.UI {
	rows := make([]app.UI, 0, len(p.imports))
	for _, imported := range p.imports {
		libraryID, number := imported.LibraryID, strconv.Itoa(imported.CommitNumber)
		body := []app.UI{
			app.Span().
				Attr("data-import-library", libraryID.String()).
				Style("font-size", "12px").
				Style("font-weight", "600").
				Style("color", "var(--text)").
				Title(p.libraryName(libraryID)).
				Text(p.libraryName(libraryID)),
			app.Div().
				Attr("data-import-provenance", number).
				Style("color", "var(--text-muted)").
				Style("font-size", "11px").
				Text(p.importProvenance(imported)),
		}
		if len(imported.Labels) > 0 {
			chips := make([]app.UI, len(imported.Labels))
			for i, label := range imported.Labels {
				chips[i] = savedstates.LabelChip(label)
			}
			body = append(body, app.Div().
				Style("display", "flex").
				Style("flex-wrap", "wrap").
				Style("gap", "4px").
				Style("margin-top", "2px").
				Body(chips...))
		}
		rows = append(rows, app.Div().
			Class("library-import").
			Attr("data-import-row", libraryID.String()).
			Style("display", "flex").
			Style("align-items", "flex-start").
			Style("gap", "4px").
			Style("padding", "3px 0").
			Body(
				app.Div().
					Style("flex", "1").
					Style("min-width", "0").
					Body(body...),
				rowButton("Remove", "", true, "Remove this library's import",
					func(ctx app.Context) { p.changeImport(libraryID, 0) },
					app.EventScope("import-remove/"+libraryID.String())).
					Attr("data-import-remove", libraryID.String()),
			))
	}
	if len(rows) == 0 {
		rows = append(rows, app.Div().
			Attr("data-import-none", "true").
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Text("No libraries imported yet."))
	}
	rows = append(rows, app.Button().
		Attr("data-import-open", "true").
		Class("library-import-add").
		Style("width", "100%").
		Style("margin-top", "6px").
		Style("font-size", "11px").
		Style("padding", "3px 6px").
		Style("background", "var(--input-bg)").
		Style("color", "var(--text)").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "3px").
		Style("cursor", "pointer").
		Text("Import libraries…").
		OnClick(func(ctx app.Context, e app.Event) { p.openImportDialog(ctx) }))

	return app.Div().
		Attr("data-imports-panel", "true").
		Style("position", "relative").
		Style("margin-bottom", "10px").
		Style("padding-bottom", "8px").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.H3().
				Style("margin", "0 0 6px 0").
				Style("font-size", "13px").
				Style("font-weight", "600").
				Style("color", "var(--text)").
				Style("text-transform", "uppercase").
				Style("letter-spacing", "0.5px").
				Text("Imported libraries"),
			app.Div().Body(rows...),
			app.If(p.importDialog, p.renderImportDialog),
		)
}

// ── The window ──────────────────────────────────────────────────────────────

// renderImportDialog is the Import libraries window: the label filter, every
// library of the server with what the project imports of it, and the saved
// states of the one opened, newest first, each with its Import or Switch.
func (p *Project) renderImportDialog() app.UI {
	rows := []app.UI{p.renderImportLabelFilter()}
	switch {
	case !p.librariesLoaded:
		rows = append(rows, app.P().Style("margin", "0 0 6px").Style("color", "var(--text-muted)").
			Text("Loading libraries…"))
	case len(p.libraries) == 0:
		rows = append(rows, app.P().
			Attr("data-import-no-libraries", "true").
			Style("margin", "0 0 6px").
			Text("No libraries yet."))
	default:
		for _, lib := range p.libraries {
			rows = append(rows, p.renderImportLibraryRow(lib))
			if lib.ID == p.importLibraryID {
				rows = append(rows, p.renderImportSavedStates(lib))
			}
		}
	}
	return app.Div().
		Attr("data-import-dialog", "true").
		Style("position", "fixed").
		Style("top", "60px").
		Style("left", "16px").
		Style("z-index", "30").
		Style("width", "620px").
		Style("max-height", "78vh").
		Style("overflow-y", "auto").
		Style("padding", "14px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "6px").
		Style("background", "var(--bg)").
		Style("box-shadow", "var(--toast-shadow)").
		Style("font-size", "13px").
		Body(
			app.H3().Style("margin", "0 0 8px").Text("Import libraries"),
			app.Div().Body(rows...),
			app.Div().
				Style("display", "flex").
				Style("justify-content", "flex-end").
				Style("margin-top", "10px").
				Body(app.Button().
					Style("padding", "4px 12px").
					Style("font-size", "13px").
					Style("cursor", "pointer").
					Text("Close").
					OnClick(func(ctx app.Context, e app.Event) {
						p.closeImportDialog()
						ctx.Update()
					})),
		)
}

// renderImportLabelFilter is the window's header: the label the saved states
// of the opened library are narrowed to. It never narrows the list of
// libraries. An Organization without labels has no filter and says so.
func (p *Project) renderImportLabelFilter() app.UI {
	switch {
	case !p.historyLabelsLoaded:
		return app.Div()
	case len(p.historyLabels) == 0:
		return app.Div().
			Attr("data-import-no-labels", "true").
			Style("margin-bottom", "8px").
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Text("No labels yet")
	}
	options := []app.UI{
		app.Option().Value("").Selected(p.importLabel == uuid.Nil).Text("All labels"),
	}
	for _, label := range p.historyLabels {
		options = append(options, app.Option().
			Value(label.ID.String()).
			Selected(label.ID == p.importLabel).
			Text(label.Name))
	}
	return app.Div().
		Attr("data-import-filter", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "6px").
		Style("margin-bottom", "8px").
		Body(
			app.Span().
				Style("font-size", "12px").
				Style("color", "var(--text-muted)").
				Text("Label:"),
			app.Select().
				Attr("data-import-label", "true").
				Style("font-size", "12px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				Style("border", "1px solid var(--border-input)").
				OnChange(func(ctx app.Context, e app.Event) {
					p.setImportLabel(ctx, uiutil.ParseID(ctx.JSSrc().Get("value").String()))
				}, app.EventScope("import-label")).
				Body(options...),
		)
}

// renderImportLibraryRow is one Library of the window: its name, its latest
// saved state and what the project imports of it, or "Not imported"; clicking
// it opens its saved states.
func (p *Project) renderImportLibraryRow(lib apiv0.LibraryResponse) app.UI {
	state := "Not imported"
	if imported, ok := p.importOf(lib.ID); ok {
		state = fmt.Sprintf("Imported: saved state #%d", imported.CommitNumber)
	}
	latest := "never saved"
	if lib.LatestCommit > 0 {
		latest = fmt.Sprintf("latest saved state #%d", lib.LatestCommit)
	}
	row := app.Div().
		Attr("data-import-library-row", lib.ID.String()).
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "space-between").
		Style("gap", "8px").
		Style("padding", "4px 6px").
		Style("cursor", "pointer").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Span().Body(
				app.Strong().Text(lib.Name),
				app.Span().Style("margin-left", "8px").Style("color", "var(--text-muted)").
					Style("font-size", "11px").Text(latest),
			),
			app.Span().
				Attr("data-import-state", lib.ID.String()).
				Style("font-size", "11px").
				Style("color", "var(--text-muted)").
				Text(state),
		).
		OnClick(func(ctx app.Context, e app.Event) {
			p.selectImportLibrary(ctx, lib.ID)
		}, app.EventScope("import-library/"+lib.ID.String()))
	if lib.ID == p.importLibraryID {
		row = row.Style("background", "var(--bg-elevated)").Style("font-weight", "600")
	}
	return row
}

// renderImportSavedStates is the opened library's saved states, newest first:
// each with its number, name, Author, time, labels and number of WidgetTypes,
// and the Import (a library not imported), Switch (another saved state) or
// Imported mark. A library without saved states says how to make one.
func (p *Project) renderImportSavedStates(lib apiv0.LibraryResponse) app.UI {
	rows := []app.UI{}
	switch {
	case !p.importCommitsLoaded:
		rows = append(rows, app.P().Style("margin", "4px 0").Style("color", "var(--text-muted)").
			Text("Loading saved states…"))
	case len(p.importCommits) == 0 && p.importLabel != uuid.Nil:
		rows = append(rows, app.P().Style("margin", "4px 0").Text("No saved state carries this label."))
	case len(p.importCommits) == 0:
		rows = append(rows, app.P().
			Attr("data-import-no-commits", "true").
			Style("margin", "4px 0").
			Text("No saved states: save the library first."))
	default:
		for _, saved := range p.importCommits {
			rows = append(rows, p.renderImportSavedStateRow(lib, saved))
		}
	}
	return app.Div().
		Attr("data-import-saved-states", lib.ID.String()).
		Style("padding", "2px 6px").
		Style("background", "var(--bg-elevated)").
		Style("border-bottom", "1px solid var(--border)").
		Body(rows...)
}

// renderImportSavedStateRow is one saved state of the opened library: what it
// is, its labels and its one action.
func (p *Project) renderImportSavedStateRow(lib apiv0.LibraryResponse, saved apiv0.CommitResponse) app.UI {
	number := strconv.Itoa(saved.Number)
	imported, hasImport := p.importOf(lib.ID)
	body := []app.UI{
		app.Strong().Text(fmt.Sprintf("#%d ", saved.Number)),
		app.Span().Text(saved.Title),
		app.Span().Style("margin-left", "8px").Style("color", "var(--text-muted)").
			Text(fmt.Sprintf("%s · %s · %d widget types", saved.Author,
				uiutil.FormatTime(saved.CreatedAt), saved.ContentCounts.WidgetTypes)),
	}
	if len(saved.Labels) > 0 {
		chips := make([]app.UI, len(saved.Labels))
		for i, label := range saved.Labels {
			chips[i] = savedstates.LabelChip(label)
		}
		body = append(body, app.Span().
			Style("display", "inline-flex").
			Style("flex-wrap", "wrap").
			Style("gap", "4px").
			Style("margin-left", "8px").
			Body(chips...))
	}
	var control app.UI
	switch {
	case hasImport && imported.CommitNumber == saved.Number:
		control = app.Span().
			Attr("data-import-current", number).
			Style("padding", "1px 7px").
			Style("font-size", "11px").
			Style("font-weight", "600").
			Style("color", "var(--accent)").
			Text("Imported")
	default:
		label := "Import"
		title := fmt.Sprintf("Import saved state %d of this library", saved.Number)
		if hasImport {
			label, title = "Switch", fmt.Sprintf("Switch the import to saved state %d", saved.Number)
		}
		libraryID, commit := lib.ID, saved.Number
		control = rowButton(label, "accent", true, title,
			func(ctx app.Context) { p.changeImport(libraryID, commit) },
			app.EventScope("import-commit/"+libraryID.String()+"/"+number)).
			Attr("data-import-commit", number)
	}
	return app.Div().
		Attr("data-import-saved-state", number).
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "space-between").
		Style("gap", "8px").
		Style("padding", "4px 0").
		Body(
			app.Span().Style("min-width", "0").Body(body...),
			app.Span().Body(control),
		)
}
