package apiclient

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// Labels returns the labels of the server's Organization, ordered by name
// ignoring case (task 85_2; ADR 0025).
func (c *Client) Labels(ctx context.Context) ([]apiv0.LabelResponse, error) {
	var resp apiv0.GetLabelsResponse
	err := c.do(ctx, http.MethodGet, apiv0.LabelPath, nil, http.StatusOK, &resp)
	return resp.Labels, err
}

// FindLabelByName returns the label with the given name, ignoring case, or
// false if there is none: names are unique in the Organization without regard
// to case.
func (c *Client) FindLabelByName(ctx context.Context, name string) (apiv0.LabelResponse, bool, error) {
	labels, err := c.Labels(ctx)
	if err != nil {
		return apiv0.LabelResponse{}, false, err
	}
	for _, l := range labels {
		if strings.EqualFold(l.Name, name) {
			return l, true, nil
		}
	}
	return apiv0.LabelResponse{}, false, nil
}

// CreateLabel creates a label with a name and an optional #RRGGBB color ("":
// the theme's neutral color). The error matches ErrConflict if the name is
// taken.
func (c *Client) CreateLabel(ctx context.Context, name, color string) (apiv0.LabelResponse, error) {
	var resp apiv0.LabelResponse
	err := c.do(ctx, http.MethodPost, apiv0.LabelPath,
		apiv0.CreateLabelRequest{Name: name, Color: color}, http.StatusCreated, &resp)
	return resp, err
}

// UpdateLabel changes the name and/or the color of a label. A nil name or
// color keeps the current one; a pointer to an empty color clears it. The
// error matches ErrConflict if the name is taken and ErrNotFound if the label
// is gone.
func (c *Client) UpdateLabel(ctx context.Context, id uuid.UUID, name, color *string) (apiv0.LabelResponse, error) {
	var resp apiv0.LabelResponse
	err := c.do(ctx, http.MethodPatch, apiv0.LabelPathOf(id.String()),
		apiv0.UpdateLabelRequest{Name: name, Color: color}, http.StatusOK, &resp)
	return resp, err
}

// DeleteLabel removes a label with its assignments; a label that is gone
// matches ErrNotFound (task 85_2; task 85_3).
func (c *Client) DeleteLabel(ctx context.Context, id uuid.UUID) error {
	return c.do(ctx, http.MethodDelete, apiv0.LabelPathOf(id.String()), nil, http.StatusNoContent, nil)
}
