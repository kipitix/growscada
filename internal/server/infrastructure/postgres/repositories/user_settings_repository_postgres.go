package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/settings"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// settingsKey names one per-user setting: the key of its row and the codec of
// its payload. A new setting is a new key with its own codec, not a new table
// (ADR 0024).
type settingsKey struct {
	name   string
	encode func(settings.Order) ([]byte, error)
	decode func([]byte) (settings.Order, error)
}

var (
	// libraryOrderKey is the order of the libraries in the selection page
	// (task 81).
	libraryOrderKey = settingsKey{"library_order", encodeLibraryOrder, decodeLibraryOrder}
	// projectOrderKey is the order of the projects in the selection page
	// (task 83).
	projectOrderKey = settingsKey{"project_order", encodeProjectOrder, decodeProjectOrder}
)

// userSettingsRepositoryPostgres keeps public.user_settings: one row per
// person, Organization and key. It writes no outbox row: a setting records no
// event (ADR 0008, ADR 0024).
type userSettingsRepositoryPostgres struct {
	db *sql.DB
}

var _ settings.Repository = (*userSettingsRepositoryPostgres)(nil)

// NewUserSettingsRepositoryPostgres returns the repository of the server's
// per-user settings. It names the public schema alone: a setting is not one
// role's content (ADR 0011).
func NewUserSettingsRepositoryPostgres(aDb *sql.DB) settings.Repository {
	return &userSettingsRepositoryPostgres{db: aDb}
}

// FindLibraryOrder returns the library order of owner in org (see
// settings.Repository).
func (r userSettingsRepositoryPostgres) FindLibraryOrder(
	ctx context.Context,
	owner id.ID[settings.User],
	org id.ID[organization.Organization],
) (settings.Settings, bool, error) {
	return r.find(ctx, owner, org, libraryOrderKey)
}

// SaveLibraryOrder writes the library order (see settings.Repository).
func (r userSettingsRepositoryPostgres) SaveLibraryOrder(ctx context.Context, s settings.Settings) (settings.Settings, error) {
	return r.save(ctx, s, libraryOrderKey)
}

// FindProjectOrder returns the project order of owner in org (see
// settings.Repository, task 83).
func (r userSettingsRepositoryPostgres) FindProjectOrder(
	ctx context.Context,
	owner id.ID[settings.User],
	org id.ID[organization.Organization],
) (settings.Settings, bool, error) {
	return r.find(ctx, owner, org, projectOrderKey)
}

// SaveProjectOrder writes the project order (see settings.Repository).
func (r userSettingsRepositoryPostgres) SaveProjectOrder(ctx context.Context, s settings.Settings) (settings.Settings, error) {
	return r.save(ctx, s, projectOrderKey)
}

// find reads the row of one setting of owner in org.
func (r userSettingsRepositoryPostgres) find(
	ctx context.Context,
	owner id.ID[settings.User],
	org id.ID[organization.Organization],
	key settingsKey,
) (settings.Settings, bool, error) {
	var (
		rawVersion int
		payload    []byte
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT version, payload FROM public.user_settings
		 WHERE user_id = $1 AND organization_id = $2 AND key = $3`,
		owner.UUID(), org.UUID(), key.name,
	).Scan(&rawVersion, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return settings.Settings{}, false, nil
	}
	if err != nil {
		return settings.Settings{}, false, fmt.Errorf("cannot read user settings: %w", err)
	}
	order, err := key.decode(payload)
	if err != nil {
		return settings.Settings{}, false, err
	}
	ver, err := version.New(version.WithNumber[settings.Settings](rawVersion))
	if err != nil {
		return settings.Settings{}, false, fmt.Errorf("cannot create settings version: %w", err)
	}
	return settings.Reconstitute(owner, org, order, ver), true, nil
}

// save writes one setting (see settings.Repository): one statement, so one
// transaction over one row. The insert's key is the row's, and a row written
// meanwhile turns it into a conflict; the update's compare-and-swap does the
// same for a stored one.
func (r userSettingsRepositoryPostgres) save(ctx context.Context, s settings.Settings, key settingsKey) (settings.Settings, error) {
	payload, err := key.encode(s.Order())
	if err != nil {
		return settings.Settings{}, err
	}
	current := s.Version()
	if !current.IsCommitted() {
		res, err := r.db.ExecContext(ctx,
			`INSERT INTO public.user_settings (user_id, organization_id, key, payload, version)
			 VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
			s.Owner().UUID(), s.OrganizationID().UUID(), key.name, payload, version.Committed[settings.Settings]().Number(),
		)
		if err != nil {
			return settings.Settings{}, fmt.Errorf("cannot insert user settings: %w", err)
		}
		written, err := writtenOnce(res)
		if err != nil {
			return settings.Settings{}, fmt.Errorf("cannot insert user settings: %w", err)
		}
		if !written {
			return settings.Settings{}, fmt.Errorf("%w: settings of %s in %s exist already",
				settings.ErrSettingsConflict, s.Owner(), s.OrganizationID())
		}
		return settings.Reconstitute(s.Owner(), s.OrganizationID(), s.Order(), version.Committed[settings.Settings]()), nil
	}

	next := current.Next()
	res, err := r.db.ExecContext(ctx,
		`UPDATE public.user_settings SET payload = $1, version = $2, updated_at = now()
		 WHERE user_id = $3 AND organization_id = $4 AND key = $5 AND version = $6`,
		payload, next.Number(), s.Owner().UUID(), s.OrganizationID().UUID(), key.name, current.Number(),
	)
	if err != nil {
		return settings.Settings{}, fmt.Errorf("cannot update user settings: %w", err)
	}
	written, err := writtenOnce(res)
	if err != nil {
		return settings.Settings{}, fmt.Errorf("cannot update user settings: %w", err)
	}
	if !written {
		return settings.Settings{}, fmt.Errorf("%w: settings of %s in %s are not at version %s",
			settings.ErrSettingsConflict, s.Owner(), s.OrganizationID(), current)
	}
	return settings.Reconstitute(s.Owner(), s.OrganizationID(), s.Order(), next), nil
}

// writtenOnce reports whether a statement wrote its row. A statement that wrote
// none lost the race: the row was inserted or changed by another writer, which
// is ErrSettingsConflict.
func writtenOnce(res sql.Result) (bool, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot get rows affected: %w", err)
	}
	return n == 1, nil
}
