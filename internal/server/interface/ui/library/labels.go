package library

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/labels"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The labels of a library's saved states (task 85_3; ADR 0025): the history
// shows a chip per label and a menu of the Organization's labels, and placing
// or taking one off is a request of its own — it states no Draft Version and
// is never a conflict with an edit.

// loadHistoryLabels (re)loads the labels of the Organization: the menu of
// every saved state of the history.
func (l *Library) loadHistoryLabels(ctx app.Context) {
	uiutil.Load(ctx, &l.historyLabelsReloader, l.client.Labels, func(ctx app.Context, list []apiv0.LabelResponse) {
		l.historyLabels = list
		ctx.Update()
	}, l.loadHistoryLabels)
}

// assignLabel marks a saved state of the history with a label and updates its
// row from the answer; nothing is shown as assigned before the server agreed.
func (l *Library) assignLabel(ctx app.Context, number int, labelID uuid.UUID) {
	draft := l.draft()
	if draft == nil {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) ([]apiv0.LabelResponse, error) {
		return draft.AssignLabel(c, number, labelID)
	}, func(ctx app.Context, list []apiv0.LabelResponse) {
		if draft != l.draft() {
			return
		}
		l.applySavedStateLabels(number, list)
		l.loadHistoryLabels(ctx) // the label's assigned count follows
		ctx.Update()
	}, nil)
}

// unassignLabel takes a label off a saved state of the history, as assignLabel
// places one.
func (l *Library) unassignLabel(ctx app.Context, number int, labelID uuid.UUID) {
	draft := l.draft()
	if draft == nil {
		return
	}
	uiutil.Call(l.compoCtx, func(c context.Context) ([]apiv0.LabelResponse, error) {
		return draft.UnassignLabel(c, number, labelID)
	}, func(ctx app.Context, list []apiv0.LabelResponse) {
		if draft != l.draft() {
			return
		}
		l.applySavedStateLabels(number, list)
		l.loadHistoryLabels(ctx)
		ctx.Update()
	}, nil)
}

// manageLabels opens the tab's Labels window, where the labels a menu offers
// are named and colored.
func (l *Library) manageLabels(ctx app.Context) {
	ctx.NewAction(labels.ActionOpen)
}

// applySavedStateLabels replaces the labels of the saved state number of the
// history with the server's answer.
func (l *Library) applySavedStateLabels(number int, list []apiv0.LabelResponse) {
	for i := range l.savedStates {
		if l.savedStates[i].Number == number {
			l.savedStates[i].Labels = list
			return
		}
	}
}
