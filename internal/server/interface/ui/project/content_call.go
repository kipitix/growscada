package project

import (
	"context"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// callContent is uiutil.Call on the content of the project shown; nothing is
// sent when none is. The outcome changes what is shown of that project, so
// once another one is shown onOK and onErr are not called: an error is only
// shown in a toast.
func callContent[T any](p *Project, call func(context.Context, *apiclient.ProjectClient) (T, error), onOK func(app.Context, T), onErr func(app.Context, error)) {
	content := p.content
	if content == nil {
		return // no project shown
	}
	uiutil.Call(p.compoCtx, func(c context.Context) (T, error) {
		return call(c, content)
	}, func(ctx app.Context, v T) {
		if content == p.content && onOK != nil {
			onOK(ctx, v)
		}
	}, func(ctx app.Context, err error) {
		if content != p.content || onErr == nil {
			uiutil.ShowError(ctx, err)
			return
		}
		onErr(ctx, err)
	})
}
