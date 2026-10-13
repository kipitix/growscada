package uiutil

import (
	"context"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
)

// Call runs call (a request to the server API) off the UI goroutine and hands
// its result back on the UI goroutine: the value to onOK (if not nil), an
// error to onErr — or, when onErr is nil, to a toast built by toast.FromError.
//
// ctx must be the component's own context (compoCtx, captured in OnMount):
// an event handler's ctx belongs to the element that fired the event, and
// go-app silently drops Dispatch once that element is unmounted, which would
// lose the result.
func Call[T any](ctx app.Context, call func(context.Context) (T, error), onOK func(app.Context, T), onErr func(app.Context, error)) {
	ctx.Async(func() {
		v, err := call(ctx)
		ctx.Dispatch(func(ctx app.Context) {
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
	})
}

// ShowError shows err, an error of an apiclient call, as a toast.
func ShowError(ctx app.Context, err error) {
	ctx.NewActionWithValue(toast.ActionAdd, toast.FromError(err))
}
