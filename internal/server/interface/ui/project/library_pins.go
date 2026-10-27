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
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Library Pins ──────────────────────────────────────────────────────────────
//
// The project uses the WidgetTypes of the Library Releases it pins, one per
// library (ADR 0007). A switch or removal is previewed first: Widgets it
// would leave without their WidgetType block it, the PortBindings it would
// break need a confirmation, and the server removes them after it
// (ADR 0012).

// libraryName returns the name of the library id, or its ID while the
// libraries are not loaded.
func (p *Project) libraryName(id uuid.UUID) string {
	if l, ok := uiutil.FindByID(p.libraries, id, uiutil.LibraryID); ok {
		return l.Name
	}
	return id.String()
}

// pinOf returns the project's pin of the library id.
func (p *Project) pinOf(id uuid.UUID) (apiv0.LibraryPinResponse, bool) {
	for _, pin := range p.pins {
		if pin.LibraryID == id {
			return pin, true
		}
	}
	return apiv0.LibraryPinResponse{}, false
}

// addPin pins the latest release of the library.
func (p *Project) addPin(libraryID uuid.UUID) {
	lib, ok := uiutil.FindByID(p.libraries, libraryID, uiutil.LibraryID)
	if !ok || lib.LatestRelease == 0 || p.content == nil {
		return
	}
	content := p.content
	uiutil.Call(p.compoCtx, func(c context.Context) (apiv0.LibraryPinResponse, error) {
		return content.PinLibrary(c, libraryID, apiv0.PutLibraryPinRequest{Release: lib.LatestRelease})
	}, func(ctx app.Context, _ apiv0.LibraryPinResponse) {
		p.loadPins(ctx)
		p.loadWidgetTypes(ctx)
	}, nil)
}

// changePin switches the pin of the library to release, or removes it for
// release 0, after its preview: orphaned Widgets refuse it, broken bindings
// are confirmed.
func (p *Project) changePin(libraryID uuid.UUID, release int) {
	pin, ok := p.pinOf(libraryID)
	if !ok || p.content == nil || pin.Release == release {
		return
	}
	content := p.content
	uiutil.Call(p.compoCtx, func(c context.Context) (apiv0.PinImpactResponse, error) {
		return content.LibraryPinImpact(c, libraryID, release)
	}, func(ctx app.Context, impact apiv0.PinImpactResponse) {
		what := fmt.Sprintf("switch %s to release %d", p.libraryName(libraryID), release)
		if release == 0 {
			what = "unpin " + p.libraryName(libraryID)
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
		p.savePin(content, libraryID, release, pin.Version)
	}, nil)
}

// savePin switches (release > 0) or removes the pin.
func (p *Project) savePin(content *apiclient.ProjectClient, libraryID uuid.UUID, release, version int) {
	uiutil.Call(p.compoCtx, func(c context.Context) (apiv0.LibraryPinResponse, error) {
		if release == 0 {
			return content.UnpinLibrary(c, libraryID)
		}
		return content.PinLibrary(c, libraryID, apiv0.PutLibraryPinRequest{Release: release, Version: version})
	}, func(ctx app.Context, _ apiv0.LibraryPinResponse) {
		p.loadPins(ctx)
		p.loadWidgetTypes(ctx)
	}, func(ctx app.Context, err error) {
		// Widgets placed since the preview: the refusal names them.
		var se *apiclient.StatusError
		if errors.As(err, &se) && len(se.Problem.OrphanedWidgets) > 0 {
			p.showOrphaned(ctx, "change the pin", se.Problem.OrphanedWidgets)
		} else {
			uiutil.ShowError(ctx, err)
		}
		p.loadPins(ctx)
	})
}

// showOrphaned tells that the change what is refused while the widgets are
// of WidgetTypes it would drop.
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

// renderPinsPanel renders the project's pins: each with its release to
// switch to and a removal, and the libraries to add.
func (p *Project) renderPinsPanel() app.UI {
	rows := make([]app.UI, 0, len(p.pins)+1)
	for _, pin := range p.pins {
		libraryID := pin.LibraryID
		latest := pin.Release
		if lib, ok := uiutil.FindByID(p.libraries, libraryID, uiutil.LibraryID); ok && lib.LatestRelease > latest {
			latest = lib.LatestRelease
		}
		options := make([]app.UI, 0, latest)
		for n := latest; n >= 1; n-- {
			options = append(options, app.Option().Value(strconv.Itoa(n)).Selected(n == pin.Release).Text("release "+strconv.Itoa(n)))
		}
		rows = append(rows, app.Div().
			Class("library-pin").
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "4px").
			Style("margin-bottom", "4px").
			Style("font-size", "12px").
			Body(
				app.Span().
					Style("flex", "1").
					Style("overflow", "hidden").
					Style("text-overflow", "ellipsis").
					Style("white-space", "nowrap").
					Title(p.libraryName(libraryID)).
					Text(p.libraryName(libraryID)),
				app.Select().
					Style("font-size", "11px").
					Style("background", "var(--input-bg)").
					Style("color", "var(--text)").
					Style("border", "1px solid var(--border-input)").
					OnChange(func(ctx app.Context, e app.Event) {
						n, err := strconv.Atoi(ctx.JSSrc().Get("value").String())
						if err == nil {
							p.changePin(libraryID, n)
						}
					}).
					Body(options...),
				app.Button().
					Style("font-size", "11px").
					Style("padding", "1px 6px").
					Style("cursor", "pointer").
					Style("border", "1px solid var(--border-input)").
					Style("border-radius", "3px").
					Style("background", "var(--bg-hover)").
					Style("color", "var(--text)").
					Title("Unpin").
					Text("✕").
					OnClick(func(ctx app.Context, e app.Event) {
						p.changePin(libraryID, 0)
					}),
			))
	}

	addable := []app.UI{app.Option().Value("").Selected(true).Text("+ Pin library…")}
	for _, lib := range p.libraries {
		if _, pinned := p.pinOf(lib.ID); pinned || lib.LatestRelease == 0 {
			continue
		}
		addable = append(addable, app.Option().Value(lib.ID.String()).Text(lib.Name+" (release "+strconv.Itoa(lib.LatestRelease)+")"))
	}
	if len(addable) > 1 {
		rows = append(rows, app.Select().
			Class("library-pin-add").
			Style("width", "100%").
			Style("font-size", "11px").
			Style("background", "var(--input-bg)").
			Style("color", "var(--text)").
			Style("border", "1px solid var(--border-input)").
			OnChange(func(ctx app.Context, e app.Event) {
				if id := uiutil.ParseID(ctx.JSSrc().Get("value").String()); id != uuid.Nil {
					p.addPin(id)
				}
				ctx.JSSrc().Set("value", "")
			}).
			Body(addable...))
	}
	if len(rows) == 0 {
		rows = append(rows, app.Div().
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Text("No library released yet."))
	}
	return app.Div().
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
				Text("Libraries"),
			app.Div().Body(rows...),
		)
}
