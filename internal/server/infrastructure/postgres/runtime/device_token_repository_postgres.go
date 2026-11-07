package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/devicetoken"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// deviceTokenRepositoryPostgres is the devicetoken.Repository over
// runtime.device_tokens, the Runtime node's own table (ADR 0011): a token is
// stored as its hash, with its fingerprint, the Device it acts for and that
// Device's Project. There is no foreign key to the copy of the Device on
// purpose: a token outlives a Deploy that drops its Device from the running
// Deployment, and sleeps until the Device is back.
type deviceTokenRepositoryPostgres struct {
	db *sql.DB
}

var _ devicetoken.Repository = (*deviceTokenRepositoryPostgres)(nil)

// NewDeviceTokenRepositoryPostgres returns the repository of the Device
// Tokens.
func NewDeviceTokenRepositoryPostgres(aDb *sql.DB) devicetoken.Repository {
	return &deviceTokenRepositoryPostgres{db: aDb}
}

const selectDeviceTokenColumns = `device_id, project_id, fingerprint, issued_at`

func (r deviceTokenRepositoryPostgres) Insert(ctx context.Context, t devicetoken.Token, tokenHash string) error {
	var inserted int
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO runtime.device_tokens (device_id, project_id, token_hash, fingerprint, issued_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (device_id) DO NOTHING
		 RETURNING 1`,
		t.DeviceID.UUID(), t.ProjectID.UUID(), tokenHash, t.Fingerprint, t.IssuedAt,
	).Scan(&inserted)
	if errors.Is(err, sql.ErrNoRows) {
		return devicetoken.ErrTokenExists
	}
	if err != nil {
		return fmt.Errorf("cannot insert the token of device %s: %w", t.DeviceID, err)
	}
	return nil
}

func (r deviceTokenRepositoryPostgres) Replace(ctx context.Context, t devicetoken.Token, tokenHash string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO runtime.device_tokens (device_id, project_id, token_hash, fingerprint, issued_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (device_id) DO UPDATE
		 SET project_id = EXCLUDED.project_id, token_hash = EXCLUDED.token_hash,
		     fingerprint = EXCLUDED.fingerprint, issued_at = EXCLUDED.issued_at`,
		t.DeviceID.UUID(), t.ProjectID.UUID(), tokenHash, t.Fingerprint, t.IssuedAt,
	)
	if err != nil {
		return fmt.Errorf("cannot replace the token of device %s: %w", t.DeviceID, err)
	}
	return nil
}

func (r deviceTokenRepositoryPostgres) FindByHash(ctx context.Context, tokenHash string) (devicetoken.Token, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+selectDeviceTokenColumns+` FROM runtime.device_tokens WHERE token_hash = $1`,
		tokenHash,
	)
	return scanDeviceToken(row.Scan)
}

func (r deviceTokenRepositoryPostgres) FindByDevice(ctx context.Context, deviceID id.ID[device.Device]) (devicetoken.Token, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+selectDeviceTokenColumns+` FROM runtime.device_tokens WHERE device_id = $1`,
		deviceID.UUID(),
	)
	return scanDeviceToken(row.Scan)
}

func (r deviceTokenRepositoryPostgres) Delete(ctx context.Context, deviceID id.ID[device.Device], projectID id.ID[project.Project]) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM runtime.device_tokens WHERE device_id = $1 AND project_id = $2`,
		deviceID.UUID(), projectID.UUID(),
	)
	if err != nil {
		return fmt.Errorf("cannot delete the token of device %s: %w", deviceID, err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot delete the token of device %s: %w", deviceID, err)
	}
	if removed == 0 {
		return devicetoken.ErrTokenNotFound
	}
	return nil
}

func (r deviceTokenRepositoryPostgres) DeleteByProject(ctx context.Context, projectID id.ID[project.Project]) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM runtime.device_tokens WHERE project_id = $1`,
		projectID.UUID(),
	); err != nil {
		return fmt.Errorf("cannot delete the tokens of project %s: %w", projectID, err)
	}
	return nil
}

// scanDeviceToken reads a row of selectDeviceTokenColumns; no row is
// ErrTokenNotFound.
func scanDeviceToken(scan func(...any) error) (devicetoken.Token, error) {
	var (
		rawDeviceID, rawProjectID uuid.UUID
		fingerprint               string
		issuedAt                  time.Time
	)
	if err := scan(&rawDeviceID, &rawProjectID, &fingerprint, &issuedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return devicetoken.Token{}, devicetoken.ErrTokenNotFound
		}
		return devicetoken.Token{}, fmt.Errorf("cannot read the device token: %w", err)
	}
	return devicetoken.Token{
		DeviceID:    id.NewID(id.IDWithUUID[device.Device](rawDeviceID)),
		ProjectID:   id.NewID(id.IDWithUUID[project.Project](rawProjectID)),
		Fingerprint: fingerprint,
		IssuedAt:    issuedAt.UTC(),
	}, nil
}
