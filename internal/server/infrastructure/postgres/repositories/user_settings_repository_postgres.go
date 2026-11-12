package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/settings"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// keyLibraryOrder is the stored key of the library order (task 81): the order
// of the libraries in the selection page. A new per-user setting is a new key
// with its own codec, not a new table (ADR 0024).
const keyLibraryOrder = "library_order"

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

// libraryOrderPayload is the stored shape of the library_order setting: a
// document, so the key can gain fields later without a migration.
type libraryOrderPayload struct {
	LibraryIDs []uuid.UUID `json:"library_ids"`
}

// encodeLibraryOrder encodes the order as the row's payload.
func encodeLibraryOrder(order settings.Order) ([]byte, error) {
	payload, err := json.Marshal(libraryOrderPayload{LibraryIDs: order.IDs()})
	if err != nil {
		return nil, fmt.Errorf("cannot encode library order: %w", err)
	}
	return payload, nil
}

// decodeLibraryOrder decodes the order of a row's payload. Duplicates and the
// zero ID are normalized away, as a client's request is.
func decodeLibraryOrder(payload []byte) (settings.Order, error) {
	var stored libraryOrderPayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return settings.Order{}, fmt.Errorf("cannot decode library order: %w", err)
	}
	return settings.NewOrder(stored.LibraryIDs), nil
}

// Find returns the settings of own in org (see settings.Repository).
func (r userSettingsRepositoryPostgres) Find(
	ctx context.Context,
	own id.ID[settings.User],
	org id.ID[organization.Organization],
) (settings.Settings, bool, error) {
	var (
		rawVersion int
		payload    []byte
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT version, payload FROM public.user_settings
		 WHERE user_id = $1 AND organization_id = $2 AND key = $3`,
		own.UUID(), org.UUID(), keyLibraryOrder,
	).Scan(&rawVersion, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return settings.Settings{}, false, nil
	}
	if err != nil {
		return settings.Settings{}, false, fmt.Errorf("cannot read user settings: %w", err)
	}
	order, err := decodeLibraryOrder(payload)
	if err != nil {
		return settings.Settings{}, false, err
	}
	ver, err := version.New(version.WithNumber[settings.Settings](rawVersion))
	if err != nil {
		return settings.Settings{}, false, fmt.Errorf("cannot create settings version: %w", err)
	}
	return settings.Reconstitute(own, org, order, ver), true, nil
}

// Save writes the settings (see settings.Repository): one statement, so one
// transaction over one row. The insert's key is the row's, and a row written
// meanwhile turns it into a conflict; the update's compare-and-swap does the
// same for a stored one.
func (r userSettingsRepositoryPostgres) Save(ctx context.Context, s settings.Settings) (settings.Settings, error) {
	payload, err := encodeLibraryOrder(s.LibraryOrder())
	if err != nil {
		return settings.Settings{}, err
	}
	current := s.Version()
	if !current.IsCommitted() {
		res, err := r.db.ExecContext(ctx,
			`INSERT INTO public.user_settings (user_id, organization_id, key, payload, version)
			 VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
			s.Owner().UUID(), s.OrganizationID().UUID(), keyLibraryOrder, payload, version.Committed[settings.Settings]().Number(),
		)
		if err != nil {
			return settings.Settings{}, fmt.Errorf("cannot insert user settings: %w", err)
		}
		written, err := res.RowsAffected()
		if err != nil {
			return settings.Settings{}, fmt.Errorf("cannot get rows affected on insert: %w", err)
		}
		if written != 1 {
			return settings.Settings{}, fmt.Errorf("%w: settings of %s in %s exist already",
				settings.ErrSettingsConflict, s.Owner(), s.OrganizationID())
		}
		return settings.Reconstitute(s.Owner(), s.OrganizationID(), s.LibraryOrder(), version.Committed[settings.Settings]()), nil
	}

	next := current.Next()
	res, err := r.db.ExecContext(ctx,
		`UPDATE public.user_settings SET payload = $1, version = $2, updated_at = now()
		 WHERE user_id = $3 AND organization_id = $4 AND key = $5 AND version = $6`,
		payload, next.Number(), s.Owner().UUID(), s.OrganizationID().UUID(), keyLibraryOrder, current.Number(),
	)
	if err != nil {
		return settings.Settings{}, fmt.Errorf("cannot update user settings: %w", err)
	}
	written, err := res.RowsAffected()
	if err != nil {
		return settings.Settings{}, fmt.Errorf("cannot get rows affected on update: %w", err)
	}
	if written != 1 {
		return settings.Settings{}, fmt.Errorf("%w: settings of %s in %s are not at version %s",
			settings.ErrSettingsConflict, s.Owner(), s.OrganizationID(), current)
	}
	return settings.Reconstitute(s.Owner(), s.OrganizationID(), s.LibraryOrder(), next), nil
}
