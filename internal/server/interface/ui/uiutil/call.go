package uiutil

import (
	"context"
	"errors"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
)

// Do runs call (a request to the server API) off the UI goroutine and hands
// its value and error to done on the UI goroutine.
//
// ctx must be the component's own context (compoCtx, captured in OnMount):
// an event handler's ctx belongs to the element that fired the event, and
// go-app silently drops Dispatch once that element is unmounted, which would
// lose the result.
func Do[T any](ctx app.Context, call func(context.Context) (T, error), done func(app.Context, T, error)) {
	ctx.Async(func() {
		v, err := call(ctx)
		ctx.Dispatch(func(ctx app.Context) { done(ctx, v, err) })
	})
}

// Call is Do split by outcome: the value to onOK (if not nil), an error to
// onErr — or, when onErr is nil, to a toast built by toast.FromError.
func Call[T any](ctx app.Context, call func(context.Context) (T, error), onOK func(app.Context, T), onErr func(app.Context, error)) {
	Do(ctx, call, func(ctx app.Context, v T, err error) {
		switch {
		case err == nil:
			if onOK != nil {
				onOK(ctx, v)
			}
		case onErr != nil:
			onErr(ctx, err)
		default:
			ShowError(ctx, err)
		}
	})
}

// Load (re)loads one server-backed list through r: unless a load is already
// in flight (it then reruns once done), it runs call, hands the value to onOK
// or shows the error, and calls reload if a rerun was requested meanwhile.
// reload is the caller's own loader, so a rerun picks up its current inputs.
func Load[T any](ctx app.Context, r *Reloader, call func(context.Context) (T, error), onOK func(app.Context, T), reload func(app.Context)) {
	LoadFor(ctx, r, func() struct{} { return struct{}{} },
		func(c context.Context, _ struct{}) (T, error) { return call(c) },
		func(ctx app.Context, _ struct{}, v T) { onOK(ctx, v) },
		reload)
}

// LoadFor is Load for a list that belongs to a key, e.g. the Widgets of the
// selected Scene: key is read when the load starts and handed to call, and
// read again when the response arrives. If it changed meanwhile the response
// (or its error) belongs to a list no longer shown: it is dropped and the
// load reruns for the new key.
func LoadFor[K comparable, T any](ctx app.Context, r *Reloader, key func() K, call func(context.Context, K) (T, error), onOK func(app.Context, K, T), reload func(app.Context)) {
	loadFor(ctx, r, key, call, onOK, func(ctx app.Context, _ K, err error) { ShowError(ctx, err) }, reload)
}

// ActionProjectGone is the go-app action fired, with the project's
// uuid.UUID as its value, when a load of the content of the project shown
// finds the project gone (404): the projects.Switcher drops it and selects
// another, so a mode shows no error for what was deleted (ADR 0006).
const ActionProjectGone = "uiutil.project-gone"

// LoadOfProject is LoadFor of a list of the content of the project shown
// (its Scenes, its Tags), keyed by the project's client (nil: none, nothing
// is loaded by then). Such a list is 404 only when the project is gone, so
// its 404 fires ActionProjectGone instead of showing an error.
func LoadOfProject[T any](ctx app.Context, r *Reloader, content func() *apiclient.ProjectClient, call func(context.Context, *apiclient.ProjectClient) (T, error), onOK func(app.Context, T), reload func(app.Context)) {
	LoadForGone(ctx, r, content, call,
		func(ctx app.Context, _ *apiclient.ProjectClient, v T) { onOK(ctx, v) },
		func(ctx app.Context, pc *apiclient.ProjectClient) {
			ctx.NewActionWithValue(ActionProjectGone, pc.ID())
		}, reload)
}

// LoadForGone is LoadFor of a list that is 404 only when its key is gone
// (e.g. the WidgetTypes of a deleted Library): its 404 calls gone with the
// key instead of showing an error.
func LoadForGone[K comparable, T any](ctx app.Context, r *Reloader, key func() K, call func(context.Context, K) (T, error), onOK func(app.Context, K, T), gone func(app.Context, K), reload func(app.Context)) {
	loadFor(ctx, r, key, call, onOK,
		func(ctx app.Context, k K, err error) {
			if errors.Is(err, apiclient.ErrNotFound) {
				gone(ctx, k)
				return
			}
			ShowError(ctx, err)
		}, reload)
}

// loadFor is LoadFor handing an error of the key's list to onErr.
func loadFor[K comparable, T any](ctx app.Context, r *Reloader, key func() K, call func(context.Context, K) (T, error), onOK func(app.Context, K, T), onErr func(app.Context, K, error), reload func(app.Context)) {
	if !r.Start() {
		return
	}
	k := key()
	Do(ctx, func(c context.Context) (T, error) { return call(c, k) }, func(ctx app.Context, v T, err error) {
		rerun := r.Done()
		switch {
		case key() != k:
			rerun = true
		case err != nil:
			onErr(ctx, k, err)
		default:
			onOK(ctx, k, v)
		}
		if rerun {
			reload(ctx)
		}
	})
}

// ShowError shows err, an error of an apiclient call, as a toast.
func ShowError(ctx app.Context, err error) {
	ctx.NewActionWithValue(toast.ActionAdd, toast.FromError(err))
}
