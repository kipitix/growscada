package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/label"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
)

// constraintLabelsOrganizationName is the unique index on the names of the
// labels of an Organization, ignoring case.
const constraintLabelsOrganizationName = "uq_labels_organization_name"

type labelRepositoryPostgresImpl struct {
	db    *sql.DB
	store aggregateStore[label.Label]
}

var _ label.Repository = (*labelRepositoryPostgresImpl)(nil)

// NewLabelRepositoryPostgres returns the repository of the Labels of the
// Organization (task 85_2; ADR 0025). A label carries no Version and is not
// part of a Draft; every change is one transaction of its own with the events
// it recorded. Its assignments — the saved states it marks — are written the
// same way, each in the label's own transaction (task 85_3).
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
		found, err := scanLabel(rows.Scan, nil)
		if err != nil {
			return nil, fmt.Errorf("cannot scan label: %w", err)
		}
		labels = append(labels, found)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read labels: %w", err)
	}
	assignments, err := r.assignments(ctx, organizationID, nil)
	if err != nil {
		return nil, err
	}
	for i := range labels {
		labels[i] = label.ReconstituteLabel(labels[i].ID(), labels[i].OrganizationID(),
			labels[i].Name(), labels[i].Color(), labels[i].CreatedAt(), assignments[labels[i].ID().UUID()])
	}
	return labels, nil
}

func (r labelRepositoryPostgresImpl) FindByID(ctx context.Context, organizationID id.ID[organization.Organization], labelID id.ID[label.Label]) (label.Label, error) {
	found, err := scanLabel(r.db.QueryRowContext(ctx,
		`SELECT `+selectLabelColumns+` FROM engineering.labels WHERE id = $1 AND organization_id = $2`,
		labelID.UUID(), organizationID.UUID(),
	).Scan, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, id.NotFound(label.ErrLabelNotFound, labelID)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read label: %w", err)
	}
	assignments, err := r.assignments(ctx, organizationID, &labelID)
	if err != nil {
		return nil, err
	}
	return label.ReconstituteLabel(found.ID(), found.OrganizationID(), found.Name(),
		found.Color(), found.CreatedAt(), assignments[labelID.UUID()]), nil
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
	return label.ReconstituteLabel(l.ID(), l.OrganizationID(), l.Name(), l.Color(), l.CreatedAt(), nil), nil
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
	return label.ReconstituteLabel(l.ID(), l.OrganizationID(), l.Name(), l.Color(), l.CreatedAt(), l.Assignments()), nil
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

func (r labelRepositoryPostgresImpl) Assign(ctx context.Context, l label.Label, savedStateID id.ID[draft.SavedState]) (label.Label, error) {
	assignedAt, ok := assignmentTime(l, savedStateID)
	if !ok {
		return nil, fmt.Errorf("cannot assign label %s: the aggregate records no assignment of %s", l.ID(), savedStateID)
	}
	err := r.assignmentTx(ctx, l, func(tx *sql.Tx) ([]event.Event, error) {
		if err := lockSavedStateForAssignment(ctx, tx, savedStateID); err != nil {
			return nil, err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.saved_state_labels (label_id, saved_state_id, assigned_at)
			 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			l.ID().UUID(), savedStateID.UUID(), assignedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("cannot insert the assignment of label %s: %w", l.ID(), err)
		}
		inserted, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on insert: %w", err)
		}
		if inserted == 0 {
			// The mark was there: this is a repeat, and a repeat records no
			// event (task 85_3; ADR 0025).
			return nil, nil
		}
		return l.PendingEvents(), nil
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (r labelRepositoryPostgresImpl) Unassign(ctx context.Context, l label.Label, savedStateID id.ID[draft.SavedState]) (label.Label, error) {
	err := r.assignmentTx(ctx, l, func(tx *sql.Tx) ([]event.Event, error) {
		res, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.saved_state_labels WHERE label_id = $1 AND saved_state_id = $2`,
			l.ID().UUID(), savedStateID.UUID(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot delete the assignment of label %s: %w", l.ID(), err)
		}
		deleted, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on delete: %w", err)
		}
		if deleted == 0 {
			return nil, nil
		}
		return l.PendingEvents(), nil
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (r labelRepositoryPostgresImpl) LabelsOfScope(ctx context.Context, organizationID id.ID[organization.Organization], scope draft.Scope) (map[int][]label.Label, error) {
	labels, err := r.FindAll(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	byNumber := map[int][]label.Label{}
	for _, l := range labels {
		for _, assignment := range l.Assignments() {
			if assignment.Scope() != scope {
				continue
			}
			byNumber[assignment.SavedStateNumber()] = append(byNumber[assignment.SavedStateNumber()], l)
		}
	}
	return byNumber, nil
}

// assignmentTx runs one write of a label's assignments: it locks the label's
// row of the Organization, runs write and appends the events write reports in
// the same transaction. A label that is gone is ErrLabelNotFound, so deleting
// a label and marking a saved state serialize on its row.
func (r labelRepositoryPostgresImpl) assignmentTx(ctx context.Context, l label.Label, write func(*sql.Tx) ([]event.Event, error)) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var locked int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM engineering.labels WHERE id = $1 AND organization_id = $2 FOR UPDATE`,
		l.ID().UUID(), l.OrganizationID().UUID(),
	).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return id.NotFound(label.ErrLabelNotFound, l.ID())
	}
	if err != nil {
		return fmt.Errorf("cannot lock label %s: %w", l.ID(), err)
	}
	events, err := write(tx)
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, events); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit the assignments of label %s: %w", l.ID(), err)
	}
	if len(events) > 0 && r.store.onCommit != nil {
		r.store.onCommit()
	}
	return nil
}

// lockSavedStateForAssignment keeps a saved state from being removed while its
// mark is written: deleting a Project or a Library deletes its saved states, so
// the insert would fail on the foreign key otherwise. A saved state this server
// lacks is draft.ErrSavedStateNotFound.
func lockSavedStateForAssignment(ctx context.Context, tx *sql.Tx, savedStateID id.ID[draft.SavedState]) error {
	var locked int
	err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM engineering.saved_states WHERE id = $1 FOR SHARE`, savedStateID.UUID()).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", draft.ErrSavedStateNotFound, savedStateID)
	}
	if err != nil {
		return fmt.Errorf("cannot lock saved state %s: %w", savedStateID, err)
	}
	return nil
}

// assignmentTime is the moment the label marks savedStateID, as the aggregate
// recorded it.
func assignmentTime(l label.Label, savedStateID id.ID[draft.SavedState]) (time.Time, bool) {
	for _, assignment := range l.Assignments() {
		if assignment.SavedStateID() == savedStateID {
			return assignment.AssignedAt(), true
		}
	}
	return time.Time{}, false
}

// assignmentsByLabel reads the marks of the Organization's labels, keyed by
// the label. With labelID set, only that label's.
func (r labelRepositoryPostgresImpl) assignments(ctx context.Context, organizationID id.ID[organization.Organization], labelID *id.ID[label.Label]) (map[uuid.UUID][]label.Assignment, error) {
	query := `SELECT cl.label_id, cl.saved_state_id, cl.assigned_at, c.number, d.project_id, d.library_id
		 FROM engineering.saved_state_labels cl
		 JOIN engineering.saved_states c ON c.id = cl.saved_state_id
		 JOIN engineering.drafts d ON d.id = c.draft_id
		 JOIN engineering.labels l ON l.id = cl.label_id
		 WHERE l.organization_id = $1`
	args := []any{organizationID.UUID()}
	if labelID != nil {
		query += ` AND cl.label_id = $2`
		args = append(args, labelID.UUID())
	}
	rows, err := r.db.QueryContext(ctx, query+` ORDER BY c.number`, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read label assignments: %w", err)
	}
	defer rows.Close()

	byLabel := map[uuid.UUID][]label.Assignment{}
	for rows.Next() {
		var (
			rawLabelID, rawSavedStateID uuid.UUID
			assignedAt                  time.Time
			number                      int
			projectID, libraryID        uuid.NullUUID
		)
		if err := rows.Scan(&rawLabelID, &rawSavedStateID, &assignedAt, &number, &projectID, &libraryID); err != nil {
			return nil, fmt.Errorf("cannot scan label assignment: %w", err)
		}
		scope, err := scopeOfRow(projectID, libraryID)
		if err != nil {
			return nil, err
		}
		assignment, err := label.NewAssignment(
			id.NewID(id.IDWithUUID[draft.SavedState](rawSavedStateID)), scope, number, assignedAt)
		if err != nil {
			return nil, fmt.Errorf("cannot read label assignment: %w", err)
		}
		byLabel[rawLabelID] = append(byLabel[rawLabelID], assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read label assignments: %w", err)
	}
	return byLabel, nil
}

// scopeOfRow is the Draft's scope a saved_states row belongs to: its Draft
// names exactly one Project or Library.
func scopeOfRow(projectID, libraryID uuid.NullUUID) (draft.Scope, error) {
	switch {
	case projectID.Valid && !libraryID.Valid:
		return draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID.UUID))), nil
	case libraryID.Valid && !projectID.Valid:
		return draft.LibraryScope(id.NewID(id.IDWithUUID[library.Library](libraryID.UUID))), nil
	}
	return draft.Scope{}, fmt.Errorf("a draft names no project or library: project %v, library %v", projectID, libraryID)
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

// scanLabel reads a row of selectLabelColumns, with the assignments the caller
// loaded (nil when it needs none).
func scanLabel(scan func(...any) error, assignments []label.Assignment) (label.Label, error) {
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
		name, label.FromStorage(rawColor), createdAt, assignments,
	), nil
}
