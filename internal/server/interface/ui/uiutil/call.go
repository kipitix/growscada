package uiutil

import (
	"context"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

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
			ShowError(ctx, err)
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
