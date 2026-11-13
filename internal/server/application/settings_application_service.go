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

// SettingsService serves the per-user settings of the server: the order of the
// libraries and the order of the projects (task 81, task 83, ADR 0024). A
// setting is a person's view of the content, so it is stored apart from the
// aggregates it names and no change of it publishes an event (ADR 0008).
//
// The owner is the person the server acts for: settings.SingleUser until
// authentication exists (task 50), then the principal of the request. Nothing
// else in this service changes with it.
type SettingsService interface {
	// LibraryOrder is the library order the person stated, empty at Version 0
	// when they never reordered.
	LibraryOrder(ctx context.Context) (appdto.Order, error)

	// SetLibraryOrder stores the whole library order the client shows, stating
	// the Version it read (0 when there was none). A stale one is
	// settings.ErrSettingsConflict: a newer order is not overwritten silently.
	// Repetitions are dropped; an ID of a library that does not exist is
	// tolerated, because the order is a view of the libraries and never an
	// invariant about them (ADR 0024).
	SetLibraryOrder(ctx context.Context, version int, libraryIDs []uuid.UUID) (appdto.Order, error)

	// ProjectOrder is the project order the person stated, empty at Version 0
	// when they never reordered (task 83).
	ProjectOrder(ctx context.Context) (appdto.Order, error)

	// SetProjectOrder stores the whole project order, with the guarantees of
	// SetLibraryOrder.
	SetProjectOrder(ctx context.Context, version int, projectIDs []uuid.UUID) (appdto.Order, error)
}

// orderReader reads the settings of one person for one key; orderWriter writes
// them. Both are the repository's methods for one setting.
type orderReader func(ctx context.Context, owner id.ID[settings.User], org id.ID[organization.Organization]) (settings.Settings, bool, error)

type orderWriter func(ctx context.Context, s settings.Settings) (settings.Settings, error)

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

func (s settingsServiceImpl) LibraryOrder(ctx context.Context) (appdto.Order, error) {
	return s.order(ctx, s.repository.FindLibraryOrder)
}

func (s settingsServiceImpl) SetLibraryOrder(ctx context.Context, version int, libraryIDs []uuid.UUID) (appdto.Order, error) {
	return s.setOrder(ctx, version, libraryIDs, s.repository.FindLibraryOrder, s.repository.SaveLibraryOrder)
}

func (s settingsServiceImpl) ProjectOrder(ctx context.Context) (appdto.Order, error) {
	return s.order(ctx, s.repository.FindProjectOrder)
}

func (s settingsServiceImpl) SetProjectOrder(ctx context.Context, version int, projectIDs []uuid.UUID) (appdto.Order, error) {
	return s.setOrder(ctx, version, projectIDs, s.repository.FindProjectOrder, s.repository.SaveProjectOrder)
}

// order reads one setting of the person, empty when they never wrote it.
func (s settingsServiceImpl) order(ctx context.Context, read orderReader) (appdto.Order, error) {
	found, ok, err := read(ctx, s.owner, s.organization)
	if err != nil {
		return appdto.Order{}, fmt.Errorf("error on find settings: %w", err)
	}
	if !ok {
		return appdto.NewOrder(settings.Empty(s.owner, s.organization)), nil
	}
	return appdto.NewOrder(found), nil
}

// setOrder stores one setting at the Version the client states.
func (s settingsServiceImpl) setOrder(
	ctx context.Context,
	version int,
	ids []uuid.UUID,
	read orderReader,
	write orderWriter,
) (appdto.Order, error) {
	current, ok, err := read(ctx, s.owner, s.organization)
	if err != nil {
		return appdto.Order{}, fmt.Errorf("error on find settings: %w", err)
	}
	if !ok {
		current = settings.Empty(s.owner, s.organization)
	}
	// The stated Version is checked here so a stale write does not even try:
	// the store checks it again in its own transaction, which is what makes
	// two racing writes of one Version end with one conflict.
	if current.Version().Number() != version {
		return appdto.Order{}, fmt.Errorf("%w: settings at %d, request stated %d",
			settings.ErrSettingsConflict, current.Version().Number(), version)
	}
	saved, err := write(ctx, current.WithOrder(settings.NewOrder(ids)))
	if err != nil {
		return appdto.Order{}, fmt.Errorf("error on save settings: %w", err)
	}
	return appdto.NewOrder(saved), nil
}
