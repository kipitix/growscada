package project

import (
	"errors"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
)

// showRejectedSceneEdit shows err, the failure of an edit of a Scene or of its
// Widgets, then calls reload (if any) to put the server's state back. A
// conflict (409) means the Scene changed since the UI read it: another client
// edited it, or the server brought its Widgets into line with a changed
// WidgetType (ADR 0008). The UI then rereads the Scenes and the Widgets
// itself, without waiting for the event stream, and asks the user to repeat
// the edit; it does not reapply it. The known versions are forgotten first: if
// the server's went back, a newer one kept here would be rejected forever.
func (p *Project) showRejectedSceneEdit(ctx app.Context, err error, reload func(app.Context)) {
	prob := toast.FromError(err)
	if errors.Is(err, apiclient.ErrConflict) {
		ctx.NewActionWithValue(toast.ActionAdd, sceneConflictProblem(prob))
		p.sceneVersions.Forget()
		p.loadScenes(ctx)
		p.loadWidgets(ctx)
		return
	}
	ctx.NewActionWithValue(toast.ActionAdd, prob)
	if reload != nil {
		reload(ctx)
	}
}

// rejectedSceneEdit is showRejectedSceneEdit as the error handler of a
// uiutil.Call.
func (p *Project) rejectedSceneEdit(reload func(app.Context)) func(app.Context, error) {
	return func(ctx app.Context, err error) { p.showRejectedSceneEdit(ctx, err, reload) }
}

// sceneConflictProblem is the toast for prob, a 409 on an edit of a Scene or
// of its Widgets: it says what happened and what to do, in place of the
// server's generic conflict.
func sceneConflictProblem(prob toast.Problem) toast.Problem {
	return toast.Problem{
		Title:  "Scene changed",
		Status: http.StatusConflict,
		Detail: "The scene was changed meanwhile and has been reloaded. Repeat your edit.",
		Method: prob.Method,
	}
}
