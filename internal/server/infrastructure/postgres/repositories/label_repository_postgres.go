package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/label"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// constraintLabelsOrganizationName is the unique index on the names of a labels
// of an organization, ignoring case.
const constraintLabelsOrganizationName = "uq_labels_organization_name"

type labelRepositoryPostgresImpl struct {
	db    *sql.DB
	store aggregateStore[label.Label]
}

var _ label.Repository = (*labelRepositoryPostgresImpl)(nil)

// NewLabelRepositoryPostgres returns the repository of the Labels of the
// Organization (task 85_2; ADR 0025). A label carries no Version and is not
// part of a Draft; every change is one transaction of its own with the events
// it recorded.
func NewLabelRepositoryPostgres(aDb *sql.DB, opts ...Option) label.Repository {
	o := newOptions(opts)
	return &labelRepositoryPostgresImpl{
		db: aDb,
		store: aggregateStore[label.Label]{
			db: aDb, table: "engineering.labels",
			notFound: label.ErrLabelNotFound, conflict: label.ErrLabelNameTaken,
			onCommit: o.onCommit,
		},
	}
}

func (r labelRepositoryPostgresImpl) NextID() id.ID[label.Label] {
	return id.NewID[label.Label]()
}

func (r labelRepositoryPostgresImpl) FindAll(ctx context.Context, organizationID id.ID[organization.Organization]) ([]label.Label, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectLabelColumns+` FROM engineering.labels
		 WHERE organization_id = $1 ORDER BY lower(name), name`,
		organizationID.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot read labels: %w", err)
	}
	defer rows.Close()

	labels := []label.Label{}
	for rows.Next() {
		found, err := scanLabel(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("cannot scan label: %w", err)
		}
		labels = append(labels, found)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read labels: %w", err)
	}
	return labels, nil
}

func (r labelRepositoryPostgresImpl) FindByID(ctx context.Context, organizationID id.ID[organization.Organization], labelID id.ID[label.Label]) (label.Label, error) {
	found, err := scanLabel(r.db.QueryRowContext(ctx,
		`SELECT `+selectLabelColumns+` FROM engineering.labels WHERE id = $1 AND organization_id = $2`,
		labelID.UUID(), organizationID.UUID(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, id.NotFound(label.ErrLabelNotFound, labelID)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read label: %w", err)
	}
	return found, nil
}

func (r labelRepositoryPostgresImpl) Create(ctx context.Context, l label.Label) (label.Label, error) {
	err := r.store.inTx(ctx, l.PendingEvents(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.labels (id, organization_id, name, color, created_at)
			 VALUES ($1, $2, $3, $4, $5)`,
			l.ID().UUID(), l.OrganizationID().UUID(), l.Name().String(), nilIfEmpty(l.Color()), l.CreatedAt(),
		)
		return labelWriteError("insert new label", err)
	})
	if err != nil {
		return nil, err
	}
	return label.ReconstituteLabel(l.ID(), l.OrganizationID(), l.Name(), l.Color(), l.CreatedAt()), nil
}

func (r labelRepositoryPostgresImpl) Update(ctx context.Context, l label.Label) (label.Label, error) {
	err := r.store.inTx(ctx, l.PendingEvents(), func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE engineering.labels SET name = $1, color = $2 WHERE id = $3 AND organization_id = $4`,
			l.Name().String(), nilIfEmpty(l.Color()), l.ID().UUID(), l.OrganizationID().UUID(),
		)
		if err != nil {
			return labelWriteError("update label", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on update: %w", err)
		}
		if n != 1 {
			return id.NotFound(label.ErrLabelNotFound, l.ID())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return label.ReconstituteLabel(l.ID(), l.OrganizationID(), l.Name(), l.Color(), l.CreatedAt()), nil
}

func (r labelRepositoryPostgresImpl) Delete(ctx context.Context, l label.Label) error {
	return r.store.inTx(ctx, l.PendingEvents(), func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.labels WHERE id = $1 AND organization_id = $2`,
			l.ID().UUID(), l.OrganizationID().UUID(),
		)
		if err != nil {
			return fmt.Errorf("cannot delete label: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on delete: %w", err)
		}
		if n != 1 {
			return id.NotFound(label.ErrLabelNotFound, l.ID())
		}
		return nil
	})
}

// nilIfEmpty is the stored form of a label's color: NULL for the theme's
// neutral color.
func nilIfEmpty(c label.Color) any {
	if c.IsEmpty() {
		return nil
	}
	return c.String()
}

// labelWriteError maps a failed write of a labels row: a name another label of
// the organization holds, ignoring case, is ErrLabelNameTaken.
func labelWriteError(what string, err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err, constraintLabelsOrganizationName) {
		return label.ErrLabelNameTaken
	}
	return fmt.Errorf("cannot %s: %w", what, err)
}

const selectLabelColumns = `id, organization_id, name, COALESCE(color, ''), created_at`

// scanLabel reads a row of selectLabelColumns.
func scanLabel(scan func(...any) error) (label.Label, error) {
	var (
		rawID, rawOrgID uuid.UUID
		rawName         string
		rawColor        string
		createdAt       time.Time
	)
	if err := scan(&rawID, &rawOrgID, &rawName, &rawColor, &createdAt); err != nil {
		return nil, err
	}
	name, err := label.NewName(rawName)
	if err != nil {
		return nil, fmt.Errorf("cannot create label name: %w", err)
	}
	return label.ReconstituteLabel(
		id.NewID(id.IDWithUUID[label.Label](rawID)),
		id.NewID(id.IDWithUUID[organization.Organization](rawOrgID)),
		name, label.FromStorage(rawColor), createdAt,
	), nil
}
