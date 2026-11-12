package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/settings"
)

// SettingsService serves the per-user settings of the server: the library
// order for now (task 81, ADR 0024). A setting is a person's view of the
// content, so it is stored apart from the aggregates it names and no change of
// it publishes an event (ADR 0008).
//
// The owner is the person the server acts for: settings.SingleUser until
// authentication exists (task 50), then the principal of the request. Nothing
// else in this service changes with it.
type SettingsService interface {
	// LibraryOrder is the library order the person stated, empty at Version 0
	// when they never reordered.
	LibraryOrder(ctx context.Context) (appdto.LibraryOrder, error)

	// SetLibraryOrder stores the whole order the client shows, stating the
	// Version it read (0 when there was none). A stale one is
	// settings.ErrSettingsConflict: a newer order is not overwritten silently.
	// Repetitions are dropped; an ID of a library that does not exist is
	// tolerated, because the order is a view of the libraries and never an
	// invariant about them (ADR 0024).
	SetLibraryOrder(ctx context.Context, version int, libraryIDs []uuid.UUID) (appdto.LibraryOrder, error)
}

type settingsServiceImpl struct {
	repository   settings.Repository
	owner        id.ID[settings.User]
	organization id.ID[organization.Organization]
}

var _ SettingsService = (*settingsServiceImpl)(nil)

// NewSettingsService returns the service of the settings of the person owner
// in the Organization org. owner is the principal of the request once
// authentication exists; today it is settings.SingleUser.
func NewSettingsService(
	repository settings.Repository,
	owner id.ID[settings.User],
	org id.ID[organization.Organization],
) SettingsService {
	return &settingsServiceImpl{repository: repository, owner: owner, organization: org}
}

func (s settingsServiceImpl) LibraryOrder(ctx context.Context) (appdto.LibraryOrder, error) {
	found, ok, err := s.repository.FindLibraryOrder(ctx, s.owner, s.organization)
	if err != nil {
		return appdto.LibraryOrder{}, fmt.Errorf("error on find settings: %w", err)
	}
	if !ok {
		return appdto.NewLibraryOrder(settings.Empty(s.owner, s.organization)), nil
	}
	return appdto.NewLibraryOrder(found), nil
}

func (s settingsServiceImpl) SetLibraryOrder(ctx context.Context, version int, libraryIDs []uuid.UUID) (appdto.LibraryOrder, error) {
	current, ok, err := s.repository.FindLibraryOrder(ctx, s.owner, s.organization)
	if err != nil {
		return appdto.LibraryOrder{}, fmt.Errorf("error on find settings: %w", err)
	}
	if !ok {
		current = settings.Empty(s.owner, s.organization)
	}
	// The stated Version is checked here so a stale write does not even try:
	// the store checks it again in its own transaction, which is what makes
	// two racing writes of one Version end with one conflict.
	if current.Version().Number() != version {
		return appdto.LibraryOrder{}, fmt.Errorf("%w: settings at %d, request stated %d",
			settings.ErrSettingsConflict, current.Version().Number(), version)
	}
	saved, err := s.repository.SaveLibraryOrder(ctx, current.WithLibraryOrder(settings.NewOrder(libraryIDs)))
	if err != nil {
		return appdto.LibraryOrder{}, fmt.Errorf("error on save settings: %w", err)
	}
	return appdto.NewLibraryOrder(saved), nil
}
