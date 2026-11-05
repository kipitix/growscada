package library

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

// The EditLock of the Library's Draft (ADR 0015): the row of the library
// column, Check out and Check in, the takeover, and the read-only Draft
// without the lock. The key of the tab comes from the client the UI is built
// with (root keeps it in sessionStorage); the name the user declared is kept
// in LocalStorage — the same entry the Project mode uses — so the next Check
// out does not ask it again.

// editLockNameKey is the LocalStorage key of the declared holder name.
const editLockNameKey = "editlock:name"

// readOnlyHint is what a disabled edit says when the Draft is not held.
const readOnlyHint = "Check the Draft out to edit it"

// readOnlyTitle is the hover text of a control disabled by the read-only
// Draft: its own title, then why it is disabled.
func readOnlyTitle(title string) string { return title + " — " + readOnlyHint }

// editLockCheckInTimeout bounds the Check in a switch of library sends to the
// Draft left: the switch does not wait for it.
const editLockCheckInTimeout = 3 * time.Second

// lockMine reports whether this tab holds the selected library's EditLock.
func (l *Library) lockMine() bool {
	return l.editLock.Held && l.editLock.Holder != nil && l.editLock.Holder.Key == l.tabKey
}

// lockReadOnly reports whether the Draft may not be edited now: no lock, or
// another client's. A lock not read yet is read-only: nothing is edited
// before the state is known.
func (l *Library) lockReadOnly() bool { return !l.lockMine() }

// readOnlyReason explains why the Draft is read-only.
func (l *Library) readOnlyReason() string {
	if l.editLock.Held && l.editLock.Holder != nil && !l.lockMine() {
		return fmt.Sprintf("Read-only: %s is editing this library. Take the lock over to edit it.", l.editLock.Holder.Name)
	}
	return "Read-only: check the Draft out to edit it."
}

// loadEditLock reads the state of the selected library's EditLock. A failed
// read is not a toast: the lock is coordination, and the row keeps what it
// showed; a Check out, or the next event, brings the truth.
func (l *Library) loadEditLock(ctx app.Context) {
	if l.libraryID == uuid.Nil || l.draft() == nil {
		l.editLock, l.lockLoaded = apiv0.EditLockResponse{}, false
		return
	}
	lib := l.draft()
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.EditLockResponse, error) {
		return lib.StateEditLock(c)
	}, func(ctx app.Context, state apiv0.EditLockResponse) {
		l.applyEditLock(ctx, state)
	}, func(ctx app.Context, err error) {})
}

// applyEditLock stores the state and tells when this tab lost the lock it
// held: a takeover by another client, or its expiry. The Draft becomes
// read-only with the toast. A Check in of this tab raises no toast: its event
// may arrive before the answer to the request, and the loss is the tab's own.
func (l *Library) applyEditLock(ctx app.Context, state apiv0.EditLockResponse) {
	wasMine := l.lockMine()
	l.editLock, l.lockLoaded = state, true
	if !wasMine || l.lockMine() || l.checkingOut {
		return
	}
	ctx.NewActionWithValue(toast.ActionAdd, toast.ClientError(errors.New(
		"You no longer hold the lock of this library: it was taken over or expired. The Draft is read-only.")))
}

// checkOut takes the Draft, asking the name on the first use. The name is
// kept in LocalStorage for the next time.
func (l *Library) checkOut(ctx app.Context) {
	if l.checkingOut || l.draft() == nil {
		return
	}
	if l.lockName == "" {
		l.askingName = true
		ctx.Update()
		return
	}
	l.doCheckOut(ctx, l.lockName)
}

// confirmLockName is the OK of the name input: it remembers the name and
// checks the Draft out with it.
func (l *Library) confirmLockName(ctx app.Context) {
	name := l.lockNameInput
	if name == "" {
		return
	}
	l.askingName = false
	l.lockNameInput = ""
	l.doCheckOut(ctx, name)
}

func (l *Library) doCheckOut(ctx app.Context, name string) {
	lib := l.draft()
	if lib == nil {
		return
	}
	l.checkingOut = true
	l.lockName = name
	ctx.LocalStorage().Set(editLockNameKey, name)
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.EditLockResponse, error) {
		return lib.CheckOutEditLock(c, name)
	}, func(ctx app.Context, state apiv0.EditLockResponse) {
		l.checkingOut = false
		l.applyEditLock(ctx, state)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		l.checkingOut = false
		uiutil.ShowError(ctx, err)
		l.loadEditLock(ctx)
		ctx.Update()
	})
}

// checkIn gives the Draft back, leaving it as it is.
func (l *Library) checkIn(ctx app.Context) {
	lib := l.draft()
	if l.checkingOut || lib == nil {
		return
	}
	l.checkingOut = true
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.EditLockResponse, error) {
		return lib.CheckInEditLock(c)
	}, func(ctx app.Context, state apiv0.EditLockResponse) {
		l.checkingOut = false
		l.editLock, l.lockLoaded = state, true
		ctx.Update()
	}, func(ctx app.Context, err error) {
		l.checkingOut = false
		uiutil.ShowError(ctx, err)
		ctx.Update()
	})
}

// releaseEditLock sends a Check in for the library id in the background: it
// is what a switch of library does to the Draft left. A refusal or a network
// failure is a toast, and the switch goes on; a lock another client holds, or
// a library gone, is not a failure.
func (l *Library) releaseEditLock(id uuid.UUID) {
	if id == uuid.Nil || l.client == nil {
		return
	}
	content := l.client.Library(id)
	uiutil.Call(l.compoCtx, func(c context.Context) (struct{}, error) {
		ctx, cancel := context.WithTimeout(c, editLockCheckInTimeout)
		defer cancel()
		_, err := content.CheckInEditLock(ctx)
		return struct{}{}, err
	}, func(app.Context, struct{}) {}, func(ctx app.Context, err error) {
		if err != nil && !errors.Is(err, apiclient.ErrConflict) && !errors.Is(err, apiclient.ErrNotFound) {
			uiutil.ShowError(ctx, err)
		}
	})
}

// openTakeOver asks to confirm taking the Draft from its holder.
func (l *Library) openTakeOver(ctx app.Context) {
	if !l.editLock.Held || l.lockMine() {
		return
	}
	l.takeOverDialog = true
	if l.lockName == "" {
		// The takeover declares a name too: ask it first.
		l.askingName = true
	}
	ctx.Update()
}

// doTakeOver takes the Draft over with the version last seen; a stale one is
// the server's 409, a toast, and the state is reread. On success the content
// is reloaded as the new editor sees it.
func (l *Library) doTakeOver(ctx app.Context) {
	name := l.lockName
	if name == "" {
		name = l.lockNameInput
	}
	lib := l.draft()
	if name == "" || l.checkingOut || lib == nil {
		return
	}
	l.checkingOut = true
	version := l.editLock.Version
	l.takeOverDialog = false
	l.askingName = false
	l.lockName = name
	ctx.LocalStorage().Set(editLockNameKey, name)
	uiutil.Call(l.compoCtx, func(c context.Context) (apiv0.EditLockResponse, error) {
		return lib.TakeOverEditLock(c, name, version)
	}, func(ctx app.Context, state apiv0.EditLockResponse) {
		l.checkingOut = false
		l.editLock, l.lockLoaded = state, true
		l.loadLibraries(ctx)
		l.loadList(ctx)
		l.loadReleases(ctx)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		l.checkingOut = false
		uiutil.ShowError(ctx, err)
		l.loadEditLock(ctx)
		ctx.Update()
	})
}

// cancelTakeOver closes the takeover confirmation.
func (l *Library) cancelTakeOver() {
	l.takeOverDialog = false
}

// lockHolderSince renders the holder's time as HH:MM in the local zone.
func lockHolderSince(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.Local().Format("15:04")
}

// renderEditLockBar is the row above the libraries: who edits the selected
// Draft, Check out, Check in and the takeover, with the read-only hint.
func (l *Library) renderEditLockBar() app.UI {
	if l.libraryID == uuid.Nil {
		return app.Div()
	}
	if l.takeOverDialog {
		return l.renderTakeOverDialog()
	}
	if l.askingName {
		return app.Div().
			Attr("data-edit-lock", "name").
			Style("display", "flex").
			Style("flex-wrap", "wrap").
			Style("align-items", "center").
			Style("gap", "4px").
			Style("margin-bottom", "4px").
			Style("font-size", "12px").
			Body(
				app.Span().Style("color", "var(--text-2)").Text("Your name:"),
				app.Input().
					Type("text").
					Value(l.lockNameInput).
					Attr("data-edit-lock-name", "true").
					Style("font-size", "12px").
					Style("padding", "1px 4px").
					Style("border", "1px solid var(--border-input)").
					Style("border-radius", "3px").
					Style("width", "100%").
					OnInput(func(ctx app.Context, e app.Event) {
						l.lockNameInput = ctx.JSSrc().Get("value").String()
					}),
				l.lockButton("OK", "Check the Draft out", func(ctx app.Context) {
					l.confirmLockName(ctx)
				}),
				l.lockButton("Cancel", "Cancel", func(ctx app.Context) {
					l.askingName = false
					l.lockNameInput = ""
					l.takeOverDialog = false
					ctx.Update()
				}),
			)
	}

	var body []app.UI
	switch {
	case !l.lockLoaded:
		body = append(body, app.Span().
			Attr("data-edit-lock", "loading").
			Style("font-size", "12px").Style("color", "var(--text-muted)").
			Text("EditLock…"))
	case l.lockMine():
		body = append(body,
			app.Span().
				Attr("data-edit-lock", "mine").
				Style("font-size", "12px").Style("color", "var(--accent)").
				Text("Editing as "+l.editLock.Holder.Name),
			l.lockButton("Check in", "Give the Draft back, leaving it as it is", l.checkIn),
		)
	case l.editLock.Held:
		body = append(body,
			app.Span().
				Attr("data-edit-lock", "other").
				Style("font-size", "12px").Style("color", "var(--toast-warn-title)").
				Text(fmt.Sprintf("%s is editing since %s", l.editLock.Holder.Name, lockHolderSince(l.editLock.Holder.Since))),
			l.lockButton("Take over", "Take the Draft from "+l.editLock.Holder.Name, l.openTakeOver),
		)
	default:
		body = append(body,
			app.Span().
				Attr("data-edit-lock", "free").
				Style("font-size", "12px").Style("color", "var(--text-2)").
				Text("Read-only"),
			l.lockButton("Check out", "Take the Draft for editing", l.checkOut),
		)
	}
	if l.lockReadOnly() {
		body = append(body, app.Span().
			Attr("data-read-only", "true").
			Style("font-size", "11px").Style("color", "var(--text-muted)").
			Text(l.readOnlyReason()))
	}
	return app.Div().
		Attr("data-edit-lock-bar", "true").
		Style("display", "flex").
		Style("flex-wrap", "wrap").
		Style("align-items", "center").
		Style("gap", "6px").
		Style("position", "relative").
		Style("margin-bottom", "6px").
		Body(body...)
}

// lockButton is a small button of the lock row.
func (l *Library) lockButton(label, title string, onClick func(app.Context)) app.UI {
	return app.Button().
		Title(title).
		Disabled(l.checkingOut).
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
func (l *Library) renderTakeOverDialog() app.UI {
	holder := l.editLock.Holder
	name, since := "another client", ""
	if holder != nil {
		name, since = holder.Name, lockHolderSince(holder.Since)
	}
	rows := []app.UI{
		app.P().
			Style("margin", "0 0 8px").
			Text(fmt.Sprintf("%s has been editing this library since %s. Take it over?", name, since)),
	}
	if l.askingName || l.lockName == "" {
		rows = append(rows, app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "6px").
			Style("margin", "0 0 8px").
			Body(
				app.Span().Style("color", "var(--text-2)").Text("Your name:"),
				app.Input().
					Type("text").
					Value(l.lockNameInput).
					Attr("data-edit-lock-name", "true").
					Style("font-size", "12px").
					Style("padding", "1px 4px").
					Style("border", "1px solid var(--border-input)").
					Style("border-radius", "3px").
					OnInput(func(ctx app.Context, e app.Event) {
						l.lockNameInput = ctx.JSSrc().Get("value").String()
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
					l.lockButton("Cancel", "Leave the Draft to its holder", func(ctx app.Context) {
						l.cancelTakeOver()
						ctx.Update()
					}),
					l.lockButton("Take over", "Take the Draft for editing", func(ctx app.Context) {
						l.doTakeOver(ctx)
					}),
				),
		)
}

// tabKeyOf is the key of this tab: the client the API client was built with.
func tabKeyOf(client *apiclient.Client) uuid.UUID {
	if client == nil {
		return uuid.Nil
	}
	return uiutil.ParseID(client.ClientKey())
}
