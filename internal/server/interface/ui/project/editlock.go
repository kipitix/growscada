package project

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The EditLock of the Project's Draft (ADR 0015): the row in the project's
// header, Check out and Check in, the takeover, and the read-only Draft
// without the lock. The key of the tab comes from the client the UI is built
// with (root keeps it in sessionStorage); the name the user declared is kept
// in LocalStorage, so the next Check out does not ask it again.

// editLockNameKey is the LocalStorage key of the declared holder name.
const editLockNameKey = "editlock:name"

// readOnlyHint is what a disabled edit says when the Draft is not held.
const readOnlyHint = "Check the Draft out to edit it"

// readOnlyTitle is the hover text of a control disabled by the read-only
// Draft: its own title, then why it is disabled.
func readOnlyTitle(titles ...string) string {
	if len(titles) == 0 {
		return readOnlyHint
	}
	return titles[0] + " — " + readOnlyHint
}

// lockMine reports whether this tab holds the Draft's EditLock.
func (p *Project) lockMine() bool {
	return p.editLock.Held && p.editLock.Holder != nil && p.editLock.Holder.Key == p.tabKey
}

// readOnly reports whether the Draft may not be edited now: no lock, or
// another client's. A lock not read yet is read-only: nothing is edited
// before the state is known.
func (p *Project) readOnly() bool { return !p.lockMine() }

// refuseReadOnly tells that the Draft is read-only and reports whether it is:
// every mutating action starts with it.
func (p *Project) refuseReadOnly(ctx app.Context) bool {
	if !p.readOnly() {
		return false
	}
	ctx.NewActionWithValue(toast.ActionAdd, toast.ClientError(errors.New(p.readOnlyReason())))
	return true
}

// readOnlyReason explains why the Draft is read-only.
func (p *Project) readOnlyReason() string {
	if p.editLock.Held && p.editLock.Holder != nil && !p.lockMine() {
		return fmt.Sprintf("Read-only: %s is editing this project. Take the lock over to edit it.", p.editLock.Holder.Name)
	}
	return "Read-only: check the Draft out to edit it."
}

// loadEditLock reads the state of the Draft's EditLock. A failed read is not
// a toast: the lock is coordination, and the row keeps what it showed; a
// Check out, or the next event, brings the truth.
func (p *Project) loadEditLock(ctx app.Context) {
	if p.content == nil {
		p.editLock, p.lockLoaded = apiv0.EditLockResponse{}, false
		return
	}
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.EditLockResponse, error) {
		return pc.StateEditLock(c)
	}, func(ctx app.Context, state apiv0.EditLockResponse) {
		p.applyEditLock(ctx, state)
	}, func(ctx app.Context, err error) {})
}

// applyEditLock stores the state and tells when this tab lost the lock it
// held: a takeover by another client, or its expiry. The Draft becomes
// read-only with the toast.
func (p *Project) applyEditLock(ctx app.Context, state apiv0.EditLockResponse) {
	wasMine := p.lockMine()
	p.editLock, p.lockLoaded = state, true
	if !wasMine || p.lockMine() {
		return
	}
	ctx.NewActionWithValue(toast.ActionAdd, toast.ClientError(errors.New(
		"You no longer hold the lock of this project: it was taken over or expired. The Draft is read-only.")))
}

// checkOut takes the Draft, asking the name on the first use. The name is
// kept in LocalStorage for the next time.
func (p *Project) checkOut(ctx app.Context) {
	if p.checkingOut || p.content == nil {
		return
	}
	if p.lockName == "" {
		p.askingName = true
		ctx.Update()
		return
	}
	p.doCheckOut(ctx, p.lockName)
}

// confirmLockName is the OK of the name input: it remembers the name and
// checks the Draft out with it.
func (p *Project) confirmLockName(ctx app.Context) {
	name := p.lockNameInput
	if name == "" {
		return
	}
	p.askingName = false
	p.lockNameInput = ""
	p.doCheckOut(ctx, name)
}

func (p *Project) doCheckOut(ctx app.Context, name string) {
	p.checkingOut = true
	p.lockName = name
	ctx.LocalStorage().Set(editLockNameKey, name)
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.EditLockResponse, error) {
		return pc.CheckOutEditLock(c, name)
	}, func(ctx app.Context, state apiv0.EditLockResponse) {
		p.checkingOut = false
		p.applyEditLock(ctx, state)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.checkingOut = false
		uiutil.ShowError(ctx, err)
		p.loadEditLock(ctx)
		ctx.Update()
	})
}

// checkIn gives the Draft back, leaving it as it is.
func (p *Project) checkIn(ctx app.Context) {
	if p.checkingOut || p.content == nil {
		return
	}
	p.checkingOut = true
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.EditLockResponse, error) {
		return pc.CheckInEditLock(c)
	}, func(ctx app.Context, state apiv0.EditLockResponse) {
		p.checkingOut = false
		p.editLock, p.lockLoaded = state, true
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.checkingOut = false
		uiutil.ShowError(ctx, err)
		ctx.Update()
	})
}

// openTakeOver asks to confirm taking the Draft from its holder.
func (p *Project) openTakeOver(ctx app.Context) {
	if !p.editLock.Held || p.lockMine() {
		return
	}
	p.takeOverDialog = true
	if p.lockName == "" {
		// The takeover declares a name too: ask it first.
		p.askingName = true
	}
	ctx.Update()
}

// doTakeOver takes the Draft over with the version last seen; a stale one is
// the server's 409, a toast, and the state is reread. On success the content
// is reloaded as the new editor sees it.
func (p *Project) doTakeOver(ctx app.Context) {
	name := p.lockName
	if name == "" {
		name = p.lockNameInput
	}
	if name == "" || p.checkingOut || p.content == nil {
		return
	}
	p.checkingOut = true
	version := p.editLock.Version
	p.takeOverDialog = false
	p.askingName = false
	p.lockName = name
	ctx.LocalStorage().Set(editLockNameKey, name)
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.EditLockResponse, error) {
		return pc.TakeOverEditLock(c, name, version)
	}, func(ctx app.Context, state apiv0.EditLockResponse) {
		p.checkingOut = false
		p.editLock, p.lockLoaded = state, true
		p.loadScenes(ctx)
		p.loadWidgets(ctx)
		p.loadTags(ctx)
		p.loadPins(ctx)
		p.loadWidgetTypes(ctx)
		p.loadDeploy(ctx)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.checkingOut = false
		uiutil.ShowError(ctx, err)
		p.loadEditLock(ctx)
		ctx.Update()
	})
}

// cancelTakeOver closes the takeover confirmation.
func (p *Project) cancelTakeOver() {
	p.takeOverDialog = false
}

// lockHolderSince renders the holder's time as HH:MM in the local zone.
func lockHolderSince(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.Local().Format("15:04")
}

// renderEditLockBar is the row of the Project's header: who edits the Draft,
// Check out, Check in and the takeover, with the read-only hint.
func (p *Project) renderEditLockBar() app.UI {
	if p.takeOverDialog {
		return p.renderTakeOverDialog()
	}
	if p.askingName {
		return app.Div().
			Attr("data-edit-lock", "name").
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "6px").
			Style("font-size", "12px").
			Body(
				app.Span().Style("color", "var(--text-2)").Text("Your name:"),
				app.Input().
					Type("text").
					Value(p.lockNameInput).
					Attr("data-edit-lock-name", "true").
					Style("font-size", "12px").
					Style("padding", "1px 4px").
					Style("border", "1px solid var(--border-input)").
					Style("border-radius", "3px").
					OnInput(func(ctx app.Context, e app.Event) {
						p.lockNameInput = ctx.JSSrc().Get("value").String()
					}),
				p.lockButton("OK", "Check the Draft out", func(ctx app.Context) {
					p.confirmLockName(ctx)
				}),
				p.lockButton("Cancel", "Cancel", func(ctx app.Context) {
					p.askingName = false
					p.lockNameInput = ""
					p.takeOverDialog = false
					ctx.Update()
				}),
			)
	}

	var body []app.UI
	switch {
	case !p.lockLoaded:
		body = append(body, app.Span().
			Attr("data-edit-lock", "loading").
			Style("font-size", "12px").Style("color", "var(--text-muted)").
			Text("EditLock…"))
	case p.lockMine():
		body = append(body,
			app.Span().
				Attr("data-edit-lock", "mine").
				Style("font-size", "12px").Style("color", "var(--accent)").
				Text("Editing as "+p.editLock.Holder.Name),
			p.lockButton("Check in", "Give the Draft back, leaving it as it is", p.checkIn),
		)
	case p.editLock.Held:
		body = append(body,
			app.Span().
				Attr("data-edit-lock", "other").
				Style("font-size", "12px").Style("color", "var(--toast-warn-title)").
				Text(fmt.Sprintf("%s is editing since %s", p.editLock.Holder.Name, lockHolderSince(p.editLock.Holder.Since))),
			p.lockButton("Take over", "Take the Draft from "+p.editLock.Holder.Name, p.openTakeOver),
		)
	default:
		body = append(body,
			app.Span().
				Attr("data-edit-lock", "free").
				Style("font-size", "12px").Style("color", "var(--text-2)").
				Text("Read-only"),
			p.lockButton("Check out", "Take the Draft for editing", p.checkOut),
		)
	}
	if p.readOnly() {
		body = append(body, app.Span().
			Attr("data-read-only", "true").
			Style("font-size", "11px").Style("color", "var(--text-muted)").
			Text(p.readOnlyReason()))
	}
	return app.Div().
		Attr("data-edit-lock-bar", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "8px").
		Style("position", "relative").
		Body(body...)
}

// lockButton is a small button of the lock row.
func (p *Project) lockButton(label, title string, onClick func(app.Context)) app.UI {
	return app.Button().
		Title(title).
		Disabled(p.checkingOut).
		Style("padding", "1px 8px").
		Style("font-size", "12px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Text(label).
		OnClick(func(ctx app.Context, e app.Event) { onClick(ctx) }, app.EventScope(label))
}

// renderTakeOverDialog confirms taking the Draft from its holder, naming them
// and their time, and stating the version seen.
func (p *Project) renderTakeOverDialog() app.UI {
	holder := p.editLock.Holder
	name, since := "another client", ""
	if holder != nil {
		name, since = holder.Name, lockHolderSince(holder.Since)
	}
	rows := []app.UI{
		app.P().
			Style("margin", "0 0 8px").
			Text(fmt.Sprintf("%s has been editing this project since %s. Take it over?", name, since)),
	}
	if p.askingName || p.lockName == "" {
		rows = append(rows, app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "6px").
			Style("margin", "0 0 8px").
			Body(
				app.Span().Style("color", "var(--text-2)").Text("Your name:"),
				app.Input().
					Type("text").
					Value(p.lockNameInput).
					Attr("data-edit-lock-name", "true").
					Style("font-size", "12px").
					Style("padding", "1px 4px").
					Style("border", "1px solid var(--border-input)").
					Style("border-radius", "3px").
					OnInput(func(ctx app.Context, e app.Event) {
						p.lockNameInput = ctx.JSSrc().Get("value").String()
					}),
			))
	}
	return app.Div().
		Attr("data-takeover-dialog", "true").
		Style("position", "absolute").
		Style("top", "34px").
		Style("left", "0").
		Style("z-index", "20").
		Style("width", "360px").
		Style("padding", "12px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "6px").
		Style("background", "var(--bg)").
		Style("box-shadow", "var(--toast-shadow)").
		Style("font-size", "13px").
		Body(
			app.Div().Body(rows...),
			app.Div().
				Style("display", "flex").
				Style("justify-content", "flex-end").
				Style("gap", "8px").
				Body(
					p.lockButton("Cancel", "Leave the Draft to its holder", func(ctx app.Context) {
						p.cancelTakeOver()
						ctx.Update()
					}),
					p.lockButton("Take over", "Take the Draft for editing", func(ctx app.Context) {
						p.doTakeOver(ctx)
					}),
				),
		)
}

// tabKey is the key of this tab: the client the API client was built with.
func tabKeyOf(client *apiclient.Client) uuid.UUID {
	if client == nil {
		return uuid.Nil
	}
	return uiutil.ParseID(client.ClientKey())
}
