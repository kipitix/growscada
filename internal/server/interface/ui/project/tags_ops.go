package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

func (p *Project) createTag(ctx app.Context) {
	if p.refuseReadOnly(ctx) {
		return
	}
	req := apiv0.CreateTagRequest{Name: p.newTagName, Type: p.newTagType}
	callContent(p, func(c context.Context, content *apiclient.ProjectClient) (apiv0.CreateTagResponse, error) {
		return content.CreateTag(c, req)
	}, func(ctx app.Context, created apiv0.CreateTagResponse) {
		p.creatingTag = false
		p.newTagName = ""
		p.newTagType = ""
		p.selectedTagID = created.ID
		p.tagCreated = p.tagLists
		uiutil.StoreID(ctx, p.selectionKey(tagIDKey), created.ID)
		p.loadTags(ctx)
		p.loadDraftState(ctx)
	}, nil)
}

func (p *Project) deleteTag(ctx app.Context) {
	if p.refuseReadOnly(ctx) {
		return
	}
	deletedID := p.selectedTagID
	if deletedID == uuid.Nil {
		return
	}
	callContent(p, func(c context.Context, content *apiclient.ProjectClient) (apiv0.TagResponse, error) {
		return content.DeleteTag(c, deletedID, p.draftVersion)
	}, func(ctx app.Context, deleted apiv0.TagResponse) {
		p.observeDraftVersion(deleted.Version)
		if p.selectedTagID == deletedID {
			p.selectedTagID = uuid.Nil
			uiutil.StoreID(ctx, p.selectionKey(tagIDKey), uuid.Nil)
		}
		p.loadTags(ctx)
	}, nil)
}
