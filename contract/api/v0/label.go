package apiv0

import (
	"time"

	"github.com/google/uuid"
)

// LabelPath is the path, under PathPrefix, of the Labels of the server's
// Organization (task 85_2; ADR 0025). A label is a named mark with an optional
// color that an Engineer puts on saved states; it is not content of any
// Project or Library, and the API is server-wide while the server has one
// Organization.
const LabelPath = "/labels"

// LabelPathOf is the path of the label with the given ID.
func LabelPathOf(labelID string) string { return LabelPath + "/" + labelID }

// CreateLabelRequest is the HTTP DTO of POST /labels: a new label's name and
// its optional color. Color is #RRGGBB or empty (the theme's neutral color);
// an empty Name or a color of another form is 400, a name another label holds,
// ignoring case, is 409 label_name_taken.
type CreateLabelRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// UpdateLabelRequest is the HTTP DTO of PATCH /labels/{id}: the fields to
// change. An absent field (or null) keeps the current one; a color of "" clears
// it. The name is unique in the Organization, ignoring case, so a rename to
// another label's name is 409 label_name_taken.
type UpdateLabelRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// LabelResponse is the HTTP DTO of a Label: its identity, name, optional color
// (empty for the theme's neutral one), how many saved states it marks, and when
// it was created. The count is 0 until task 85_3 adds the assignments.
type LabelResponse struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Color         string    `json:"color"`
	AssignedCount int       `json:"assigned_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// GetLabelsResponse is the HTTP DTO of GET /labels: the Organization's labels,
// ordered by name ignoring case.
type GetLabelsResponse struct {
	Labels []LabelResponse `json:"labels"`
}
