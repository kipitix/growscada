package uiutil

import (
	"context"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

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
	if !r.Start() {
		return
	}
	Do(ctx, call, func(ctx app.Context, v T, err error) {
		if err != nil {
			ShowError(ctx, err)
		} else {
			onOK(ctx, v)
		}
		if r.Done() {
			reload(ctx)
		}
	})
}

// ShowError shows err, an error of an apiclient call, as a toast.
func ShowError(ctx app.Context, err error) {
	ctx.NewActionWithValue(toast.ActionAdd, toast.FromError(err))
}
