package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/labels"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The labels of the project's saved states (task 85_3; ADR 0025): the history
// shows a chip per label and a menu of the Organization's labels, and placing
// or taking one off is a request of its own — it states no Draft Version and
// is never a conflict with an edit.

// loadHistoryLabels (re)loads the labels of the Organization: the menu of
// every saved state of the history and the filter of the Deploy window. A
// label created, renamed, recolored or deleted elsewhere arrives here through
// the server's stream, as does an assignment; a deleted one takes a filter that
// named it back to All labels (task 85_5).
func (p *Project) loadHistoryLabels(ctx app.Context) {
	uiutil.Load(ctx, &p.historyLabelsReloader, p.client.Labels, func(ctx app.Context, list []apiv0.LabelResponse) {
		p.historyLabels, p.historyLabelsLoaded = list, true
		p.resetDeployLabelIfGone(ctx)
		ctx.Update()
	}, p.loadHistoryLabels)
}

// assignLabel marks a saved state of the history with a label and updates its
// row from the answer; nothing is shown as assigned before the server agreed.
func (p *Project) assignLabel(ctx app.Context, number int, labelID uuid.UUID) {
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.LabelResponse, error) {
		return pc.AssignLabel(c, number, labelID)
	}, func(ctx app.Context, list []apiv0.LabelResponse) {
		p.applySavedStateLabels(number, list)
		p.loadHistoryLabels(ctx) // the label's assigned count follows
		ctx.Update()
	}, nil)
}

// unassignLabel takes a label off a saved state of the history, as assignLabel
// places one.
func (p *Project) unassignLabel(ctx app.Context, number int, labelID uuid.UUID) {
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) ([]apiv0.LabelResponse, error) {
		return pc.UnassignLabel(c, number, labelID)
	}, func(ctx app.Context, list []apiv0.LabelResponse) {
		p.applySavedStateLabels(number, list)
		p.loadHistoryLabels(ctx)
		ctx.Update()
	}, nil)
}

// manageLabels opens the tab's Labels window, where the labels a menu offers
// are named and colored.
func (p *Project) manageLabels(ctx app.Context) {
	ctx.NewAction(labels.ActionOpen)
}

// applySavedStateLabels replaces the labels of the saved state number of the
// history with the server's answer.
func (p *Project) applySavedStateLabels(number int, list []apiv0.LabelResponse) {
	for i := range p.savedStates {
		if p.savedStates[i].Number == number {
			p.savedStates[i].Labels = list
			return
		}
	}
}
