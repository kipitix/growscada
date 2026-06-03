package project

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/interface/ui/toast"
)

func (p *Project) createTag(ctx app.Context) {
	name := p.newTagName
	tagType := p.newTagType
	url := p.apiServerURL + "/api/v1/tags"
	body, err := json.Marshal(createTagRequest{
		Name:    name,
		Type:    tagType,
		Value:   defaultTagValue(tagType),
		Quality: "good",
	})
	if err != nil {
		ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(err))
		return
	}
	ctx.Async(func() {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(err))
			})
			return
		}
		if resp.StatusCode >= 400 {
			prob := toast.FromHTTPError(resp)
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, prob)
			})
			return
		}
		defer resp.Body.Close()

		var result createTagResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(err))
			})
			return
		}

		ctx.Dispatch(func(ctx app.Context) {
			p.creatingTag = false
			p.newTagName = ""
			p.newTagType = ""
			p.selectedTagID = result.ID
			ctx.LocalStorage().Set("project:tagID", result.ID)
			p.loadTags(ctx)
		})
	})
}

func (p *Project) deleteTag(ctx app.Context) {
	if p.selectedTagID == "" {
		return
	}
	deletedID := p.selectedTagID
	url := p.apiServerURL + "/api/v1/tags/" + deletedID
	ctx.Async(func() {
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(err))
			})
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, toast.NetworkError(err))
			})
			return
		}
		if resp.StatusCode >= 400 {
			prob := toast.FromHTTPError(resp)
			ctx.Dispatch(func(ctx app.Context) {
				ctx.NewActionWithValue(toast.ActionAdd, prob)
			})
			return
		}
		resp.Body.Close()

		ctx.Dispatch(func(ctx app.Context) {
			if p.selectedTagID == deletedID {
				p.selectedTagID = ""
				ctx.LocalStorage().Set("project:tagID", "")
			}
			p.loadTags(ctx)
		})
	})
}

func defaultTagValue(tagType string) string {
	switch tagType {
	case "boolean":
		return "false"
	case "integer":
		return "0"
	default:
		return ""
	}
}
