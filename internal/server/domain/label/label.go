// Package label holds the Label aggregate: a named mark with an optional color
// an Engineer puts on saved states to group and filter them (task 85_2;
// ADR 0025). A Label belongs to the Organization, not to a Project or a
// Library, and it is not content: no Draft, Commit's ProjectFile or Deployment
// carries it. It owns its assignments — the marks on saved states — which are
// its entities, added in task 85_3; a rename does not need to load them, but
// the model never pretends an assignment is an aggregate of its own.
//
// Label is immutable: a change returns a new Label that also records the
// change's event. It carries no Version: a label change is never a conflict
// with an Engineer editing a Draft, and nothing states a Version to write one.
package label

import (
	"fmt"
	"slices"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// Label - aggregate representing a named mark on saved states, owned by an
// Organization (task 85_2; ADR 0025).
type Label interface {
	ID() id.ID[Label]
	OrganizationID() id.ID[organization.Organization]
	Name() Name
	// Color is the label's optional color; the zero Color is the theme's
	// neutral one.
	Color() Color
	CreatedAt() time.Time

	// Rename changes the name and records a LabelUpdatedEvent. It validates
	// the name; uniqueness in the Organization is the repository's.
	Rename(Name) (Label, error)
	// Recolor changes the color and records a LabelUpdatedEvent. An empty
	// Color clears it.
	Recolor(Color) (Label, error)
	// Delete records a LabelDeletedEvent; saving it removes the label with its
	// assignments in the same transaction (task 85_3). Like removing any
	// aggregate, it expects no Version.
	Delete() Label

	event.Recorder
	fmt.Stringer
}

type labelImpl struct {
	id             id.ID[Label]
	organizationID id.ID[organization.Organization]
	name           Name
	color          Color
	createdAt      time.Time
	pending        []event.Event
}

var _ Label = (*labelImpl)(nil)

// CreateLabel creates a new, not yet saved label of the Organization
// anOrganizationID and records a LabelCreatedEvent.
func CreateLabel(anID id.ID[Label], anOrganizationID id.ID[organization.Organization], aName Name, aColor Color, at time.Time) Label {
	return &labelImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		color:          aColor,
		createdAt:      at.UTC(),
		pending:        []event.Event{NewLabelCreatedEvent(anID)},
	}
}

// ReconstituteLabel rebuilds a label as stored; it records no event. For
// repositories.
func ReconstituteLabel(anID id.ID[Label], anOrganizationID id.ID[organization.Organization], aName Name, aColor Color, createdAt time.Time) Label {
	return &labelImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		color:          aColor,
		createdAt:      createdAt.UTC(),
	}
}

func (l *labelImpl) ID() id.ID[Label]                                 { return l.id }
func (l *labelImpl) OrganizationID() id.ID[organization.Organization] { return l.organizationID }
func (l *labelImpl) Name() Name                                       { return l.name }
func (l *labelImpl) Color() Color                                     { return l.color }
func (l *labelImpl) CreatedAt() time.Time                             { return l.createdAt }

func (l *labelImpl) Rename(aName Name) (Label, error) {
	if err := aName.validate(); err != nil {
		return nil, err
	}
	renamed := *l
	renamed.name = aName
	renamed.pending = append(slices.Clone(l.pending), NewLabelUpdatedEvent(l.id))
	return &renamed, nil
}

func (l *labelImpl) Recolor(aColor Color) (Label, error) {
	if err := aColor.validate(); err != nil {
		return nil, err
	}
	recolored := *l
	recolored.color = aColor
	recolored.pending = append(slices.Clone(l.pending), NewLabelUpdatedEvent(l.id))
	return &recolored, nil
}

func (l *labelImpl) Delete() Label {
	deleted := *l
	deleted.pending = append(slices.Clone(l.pending), NewLabelDeletedEvent(l.id))
	return &deleted
}

func (l *labelImpl) PendingEvents() []event.Event {
	return slices.Clone(l.pending)
}

// String implements [fmt.Stringer].
func (l *labelImpl) String() string {
	return fmt.Sprintf("Label: %s, color: %q, organization: %s", l.name, l.color, l.organizationID)
}
