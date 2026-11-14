package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/label"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// LabelService serves the Labels of the server's Organization: their names and
// colors (task 85_2; ADR 0025). A label is not content: the service reads no
// Draft, states no Version and writes no DraftChange. It owns its assignments
// (task 85_3), which the repository writes in the label's own transaction.
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

	// Delete removes a label with its assignments (task 85_3).
	// label.ErrLabelNotFound for a label the Organization lacks.
	Delete(ctx context.Context, labelID uuid.UUID) error
}

type labelServiceImpl struct {
	repository   label.Repository
	organization id.ID[organization.Organization]
	now          func() time.Time
}

var _ LabelService = (*labelServiceImpl)(nil)

// NewLabelService returns the service of the Organization's labels. The
// Organization is the server's own while one exists: the API is server-wide,
// and `organization_id` is written from the first day (ADR 0025).
func NewLabelService(repository label.Repository, organizationID id.ID[organization.Organization]) LabelService {
	return &labelServiceImpl{repository: repository, organization: organizationID, now: time.Now}
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

// find reads the label of the Organization; another Organization's is not
// found.
func (s labelServiceImpl) find(ctx context.Context, rawID uuid.UUID) (label.Label, error) {
	found, err := s.repository.FindByID(ctx, s.organization, id.NewID(id.IDWithUUID[label.Label](rawID)))
	if err != nil {
		return nil, fmt.Errorf("cannot find label: %w", err)
	}
	return found, nil
}
