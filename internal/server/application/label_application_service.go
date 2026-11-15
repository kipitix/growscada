package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/label"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// LabelService serves the Labels of the server's Organization: their names,
// their colors and the saved states they mark (task 85_2, task 85_3; ADR 0025).
// A label is not content: the service reads no Draft Version, states none and
// writes no DraftChange. An assignment names a saved state of a Project's or a
// Library's Draft and is written in the label's own transaction, which locks
// the label and the saved state.
type LabelService interface {
	// List returns the Organization's labels, ordered by name ignoring case.
	List(ctx context.Context) ([]appdto.Label, error)

	// Create makes a label; its name must be free in the Organization,
	// ignoring case, else label.ErrLabelNameTaken.
	Create(ctx context.Context, name, color string) (appdto.Label, error)

	// Update changes the name and/or the color of a label. A nil name or
	// color keeps the current one; a pointer to an empty color clears it.
	// label.ErrLabelNotFound for a label the Organization lacks.
	Update(ctx context.Context, labelID uuid.UUID, name, color *string) (appdto.Label, error)

	// Delete removes a label with its assignments.
	// label.ErrLabelNotFound for a label the Organization lacks.
	Delete(ctx context.Context, labelID uuid.UUID) error

	// Assign marks the saved state savedStateNumber of scope with the label and
	// returns every label the saved state carries afterwards. It is idempotent: a
	// repeat changes nothing and records no event. A Project or Library that is
	// not live is its not-found error, a saved state scope lacks is
	// draft.ErrSavedStateNotFound, a label the Organization lacks
	// label.ErrLabelNotFound; a number that is not positive is invalid input.
	Assign(ctx context.Context, scope draft.Scope, savedStateNumber int, labelID uuid.UUID) ([]appdto.Label, error)

	// Unassign takes the label off the saved state savedStateNumber of scope,
	// idempotently, and returns the labels the saved state still carries.
	Unassign(ctx context.Context, scope draft.Scope, savedStateNumber int, labelID uuid.UUID) ([]appdto.Label, error)

	// LabelsOf returns the labels of every saved state of scope that carries one,
	// keyed by the saved state's number: the read of a saved state list that shows
	// its chips (task 85_3).
	LabelsOf(ctx context.Context, scope draft.Scope) (map[int][]appdto.Label, error)
}

type labelServiceImpl struct {
	repository   label.Repository
	drafts       draft.Repository
	organization id.ID[organization.Organization]
	now          func() time.Time
}

var _ LabelService = (*labelServiceImpl)(nil)

// NewLabelService returns the service of the Organization's labels, resolving
// an assignment's saved state through the Drafts. The Organization is the
// server's own while one exists: the API is server-wide, and `organization_id`
// is written from the first day (ADR 0025).
func NewLabelService(repository label.Repository, drafts draft.Repository, organizationID id.ID[organization.Organization]) LabelService {
	return &labelServiceImpl{repository: repository, drafts: drafts, organization: organizationID, now: time.Now}
}

func (s labelServiceImpl) List(ctx context.Context) ([]appdto.Label, error) {
	labels, err := s.repository.FindAll(ctx, s.organization)
	if err != nil {
		return nil, fmt.Errorf("cannot find labels: %w", err)
	}
	return appdto.NewLabels(labels), nil
}

func (s labelServiceImpl) Create(ctx context.Context, rawName, rawColor string) (appdto.Label, error) {
	name, err := label.NewName(rawName)
	if err != nil {
		return appdto.Label{}, fmt.Errorf("cannot create label: name: %w", invalidInput(err))
	}
	color, err := label.NewColor(rawColor)
	if err != nil {
		return appdto.Label{}, fmt.Errorf("cannot create label: color: %w", invalidInput(err))
	}
	created, err := s.repository.Create(ctx,
		label.CreateLabel(s.repository.NextID(), s.organization, name, color, s.now()))
	if err != nil {
		return appdto.Label{}, fmt.Errorf("cannot save label: %w", err)
	}
	return appdto.NewLabel(created), nil
}

func (s labelServiceImpl) Update(ctx context.Context, rawID uuid.UUID, rawName, rawColor *string) (appdto.Label, error) {
	found, err := s.find(ctx, rawID)
	if err != nil {
		return appdto.Label{}, err
	}
	changed := found
	if rawName != nil && *rawName != found.Name().String() {
		name, err := label.NewName(*rawName)
		if err != nil {
			return appdto.Label{}, fmt.Errorf("cannot update label: name: %w", invalidInput(err))
		}
		if changed, err = changed.Rename(name); err != nil {
			return appdto.Label{}, fmt.Errorf("cannot update label: name: %w", invalidInput(err))
		}
	}
	if rawColor != nil && *rawColor != found.Color().String() {
		color, err := label.NewColor(*rawColor)
		if err != nil {
			return appdto.Label{}, fmt.Errorf("cannot update label: color: %w", invalidInput(err))
		}
		if changed, err = changed.Recolor(color); err != nil {
			return appdto.Label{}, fmt.Errorf("cannot update label: color: %w", invalidInput(err))
		}
	}
	if changed == found {
		return appdto.NewLabel(found), nil
	}
	saved, err := s.repository.Update(ctx, changed)
	if err != nil {
		return appdto.Label{}, fmt.Errorf("cannot save label: %w", err)
	}
	return appdto.NewLabel(saved), nil
}

func (s labelServiceImpl) Delete(ctx context.Context, rawID uuid.UUID) error {
	found, err := s.find(ctx, rawID)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, found.Delete()); err != nil {
		return fmt.Errorf("cannot delete label: %w", err)
	}
	return nil
}

func (s labelServiceImpl) Assign(ctx context.Context, scope draft.Scope, rawNumber int, rawLabelID uuid.UUID) ([]appdto.Label, error) {
	number, savedState, err := s.savedStateOf(ctx, scope, rawNumber)
	if err != nil {
		return nil, err
	}
	found, err := s.find(ctx, rawLabelID)
	if err != nil {
		return nil, err
	}
	assigned, err := found.Assign(savedState.ID(), scope, number, s.now())
	if err != nil {
		return nil, fmt.Errorf("cannot assign label: %w", invalidInput(err))
	}
	if len(assigned.PendingEvents()) > 0 {
		if _, err := s.repository.Assign(ctx, assigned, savedState.ID()); err != nil {
			return nil, fmt.Errorf("cannot save the assignment of label %s: %w", found.ID(), err)
		}
	}
	return s.labelsOfSavedState(ctx, scope, number)
}

func (s labelServiceImpl) Unassign(ctx context.Context, scope draft.Scope, rawNumber int, rawLabelID uuid.UUID) ([]appdto.Label, error) {
	number, savedState, err := s.savedStateOf(ctx, scope, rawNumber)
	if err != nil {
		return nil, err
	}
	found, err := s.find(ctx, rawLabelID)
	if err != nil {
		return nil, err
	}
	unassigned, err := found.Unassign(savedState.ID())
	if err != nil {
		return nil, fmt.Errorf("cannot unassign label: %w", invalidInput(err))
	}
	if len(unassigned.PendingEvents()) > 0 {
		if _, err := s.repository.Unassign(ctx, unassigned, savedState.ID()); err != nil {
			return nil, fmt.Errorf("cannot save the assignment of label %s: %w", found.ID(), err)
		}
	}
	return s.labelsOfSavedState(ctx, scope, number)
}

func (s labelServiceImpl) LabelsOf(ctx context.Context, scope draft.Scope) (map[int][]appdto.Label, error) {
	byNumber, err := s.repository.LabelsOfScope(ctx, s.organization, scope)
	if err != nil {
		return nil, fmt.Errorf("cannot find the labels of %s: %w", scope, err)
	}
	return appdto.NewLabelMap(byNumber), nil
}

// labelsOfSavedState is the labels the saved state savedStateNumber carries, as
// PUT and DELETE answer: the whole set, ordered by name ignoring case.
func (s labelServiceImpl) labelsOfSavedState(ctx context.Context, scope draft.Scope, savedStateNumber int) ([]appdto.Label, error) {
	byNumber, err := s.LabelsOf(ctx, scope)
	if err != nil {
		return nil, err
	}
	return byNumber[savedStateNumber], nil
}

// savedStateOf reads the saved state rawNumber of the Draft of scope, which
// checks the saved state exists in that scope and the Project or Library is
// live.
func (s labelServiceImpl) savedStateOf(ctx context.Context, scope draft.Scope, rawNumber int) (int, draft.SavedState, error) {
	if !scope.IsProject() && !scope.IsLibrary() {
		return 0, nil, invalidInput(fmt.Errorf("scope: a project or a library is required"))
	}
	number, err := savedStateNumber(rawNumber)
	if err != nil {
		return 0, nil, err
	}
	savedState, err := s.drafts.FindSavedState(ctx, scope, number)
	if err != nil {
		return 0, nil, fmt.Errorf("cannot find the saved state of %s: %w", scope, err)
	}
	return number, savedState, nil
}

// find reads the label of the Organization; another Organization's is not
// found.
func (s labelServiceImpl) find(ctx context.Context, rawID uuid.UUID) (label.Label, error) {
	found, err := s.repository.FindByID(ctx, s.organization, id.NewID(id.IDWithUUID[label.Label](rawID)))
	if err != nil {
		return nil, fmt.Errorf("cannot find label: %w", err)
	}
	return found, nil
}
