package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// SaveSavedState fixes the Draft as a saved state (see draft.Repository, task
// 72_2, ADR 0021). The content is assembled by the caller before the
// transaction; the Draft's row lock and the Version check make the snapshot the
// Draft at expected, and a write that landed during the build is a conflict.
func (r draftRepositoryPostgres) SaveSavedState(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft], title draft.SavedStateTitle, author draft.Author, content draft.SavedStateContent) (draft.SavedState, error) {
	var result draft.SavedState
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, scope, true, true)
		if err != nil {
			return nil, err
		}
		if expected.Number() != row.version {
			return nil, draft.ConflictError{Expected: expected.Number(), Current: row.version, Scope: scope}
		}
		// A Draft at its Clean Version holds what a saved state holds already:
		// there is nothing new to fix (ADR 0016, amendments 2026-11-10 and
		// 2026-11-16).
		if row.cleanVersion == row.version {
			return nil, fmt.Errorf("%w: %s", draft.ErrNothingToSave, scope)
		}
		number := row.lastSavedStateNumber + 1
		savedState, err := draft.NewSavedState(scope, id.NewID(id.IDWithUUID[draft.SavedState](uuid.New())), number,
			title, author, time.Now().UTC(), expected, content)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(savedState.Content())
		if err != nil {
			return nil, fmt.Errorf("cannot encode the content of %s: %w", savedState, err)
		}
		var storedAt time.Time
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO engineering.saved_states (id, draft_id, number, title, author, created_at, draft_version, content)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
			 RETURNING created_at`,
			savedState.ID().UUID(), row.id, number, title.String(), author.String(), savedState.CreatedAt(), expected.Number(), encoded,
		).Scan(&storedAt); err != nil {
			return nil, fmt.Errorf("cannot insert the saved state of %s: %w", scope, err)
		}
		if err := insertSavedStateImports(ctx, tx, savedState); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE engineering.drafts SET last_saved_state_number = $2, last_saved_state_draft_version = $3, clean_version = $4, updated_at = now() WHERE id = $1`,
			row.id, number, expected.Number(), expected.Number()); err != nil {
			return nil, fmt.Errorf("cannot record the last saved state of %s: %w", scope, err)
		}
		// The event was recorded when the saved state was made; the returned saved
		// state carries the time as stored.
		pending := savedState.PendingEvents()
		result, err = draft.ReconstituteSavedState(scope, savedState.ID(), number, title, author, storedAt, expected, savedState.Content())
		if err != nil {
			return nil, err
		}
		return pending, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// insertSavedStateImports writes the Library Imports a Project's ProjectFile
// carries: they are the provenance of the saved state and what keeps an
// imported Library from being deleted (task 71, task 85_6). A Library's saved
// state has neither.
func insertSavedStateImports(ctx context.Context, tx *sql.Tx, savedState draft.SavedState) error {
	file, ok := savedState.Content().ProjectFile()
	if !ok {
		return nil
	}
	for _, imported := range file.LibraryImports {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.saved_state_library_imports (saved_state_id, library_id, saved_state_number) VALUES ($1, $2, $3)`,
			savedState.ID().UUID(), imported.LibraryID, imported.SavedState,
		); err != nil {
			return fmt.Errorf("cannot insert a library import of %s: %w", savedState, err)
		}
	}
	return nil
}

// RenameSavedState changes the title of the saved state number of the Draft
// (see draft.Repository).
func (r draftRepositoryPostgres) RenameSavedState(ctx context.Context, scope draft.Scope, number int, title draft.SavedStateTitle) (draft.SavedState, error) {
	var result draft.SavedState
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		if err := checkScopeLive(ctx, tx, scope); err != nil {
			return nil, err
		}
		found, err := readSavedState(ctx, tx, scope, number)
		if err != nil {
			return nil, err
		}
		renamed := found.WithTitle(title)
		if _, err := tx.ExecContext(ctx,
			`UPDATE engineering.saved_states SET title = $2 WHERE id = $1`, found.ID().UUID(), title.String()); err != nil {
			return nil, fmt.Errorf("cannot rename %s: %w", found, err)
		}
		result = renamed
		return renamed.PendingEvents(), nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// FindSavedStates returns the Draft's saved states, newest first (see
// draft.Repository). The filter's label keeps only the saved states it marks:
// the join is in the query, never in memory.
func (r draftRepositoryPostgres) FindSavedStates(ctx context.Context, scope draft.Scope, filter draft.SavedStateFilter) ([]draft.SavedState, error) {
	if err := checkScopeLive(ctx, r.db, scope); err != nil {
		return nil, err
	}
	column, value := scopeColumn(scope)
	args := []any{value}
	query := `SELECT c.id, c.number, c.title, c.author, c.created_at, c.draft_version, c.content
		 FROM engineering.saved_states c JOIN engineering.drafts d ON d.id = c.draft_id
		 WHERE d.` + column + ` = $1`
	if filter.LabelID != uuid.Nil {
		query += ` AND EXISTS (SELECT 1 FROM engineering.saved_state_labels cl
			WHERE cl.saved_state_id = c.id AND cl.label_id = $2)`
		args = append(args, filter.LabelID)
	}
	rows, err := r.db.QueryContext(ctx, query+` ORDER BY c.number DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the saved states of %s: %w", scope, err)
	}
	defer func() { _ = rows.Close() }()
	savedStates := []draft.SavedState{}
	for rows.Next() {
		savedState, err := scanSavedStateRow(scope, rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("cannot read the saved states of %s: %w", scope, err)
		}
		savedStates = append(savedStates, savedState)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the saved states of %s: %w", scope, err)
	}
	return savedStates, nil
}

// FindLastSavedState returns the Draft's newest saved state, if it has one (see
// draft.Repository). It is one row: a Discard needs no other saved state.
func (r draftRepositoryPostgres) FindLastSavedState(ctx context.Context, scope draft.Scope) (draft.SavedState, bool, error) {
	if err := checkScopeLive(ctx, r.db, scope); err != nil {
		return nil, false, err
	}
	column, value := scopeColumn(scope)
	row := r.db.QueryRowContext(ctx,
		`SELECT c.id, c.number, c.title, c.author, c.created_at, c.draft_version, c.content
		 FROM engineering.saved_states c JOIN engineering.drafts d ON d.id = c.draft_id
		 WHERE d.`+column+` = $1 ORDER BY c.number DESC LIMIT 1`, value)
	savedState, err := scanSavedStateRow(scope, row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cannot read the last saved state of %s: %w", scope, err)
	}
	return savedState, true, nil
}

// FindSavedState returns the saved state number of the Draft (see
// draft.Repository).
func (r draftRepositoryPostgres) FindSavedState(ctx context.Context, scope draft.Scope, number int) (draft.SavedState, error) {
	if err := checkScopeLive(ctx, r.db, scope); err != nil {
		return nil, err
	}
	return readSavedState(ctx, r.db, scope, number)
}

// readSavedState reads one saved state of the Draft of scope.
func readSavedState(ctx context.Context, q queryRower, scope draft.Scope, number int) (draft.SavedState, error) {
	column, value := scopeColumn(scope)
	row := q.QueryRowContext(ctx,
		`SELECT c.id, c.number, c.title, c.author, c.created_at, c.draft_version, c.content
		 FROM engineering.saved_states c JOIN engineering.drafts d ON d.id = c.draft_id
		 WHERE d.`+column+` = $1 AND c.number = $2`, value, number)
	savedState, err := scanSavedStateRow(scope, row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %d of %s", draft.ErrSavedStateNotFound, number, scope)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read saved state %d of %s: %w", number, scope, err)
	}
	return savedState, nil
}

// scanSavedStateRow rebuilds a saved state from a row: id, number, title,
// author, created_at, draft_version and the encoded content.
func scanSavedStateRow(scope draft.Scope, scan func(...any) error) (draft.SavedState, error) {
	var (
		rawID        uuid.UUID
		number       int
		rawTitle     string
		rawAuthor    string
		createdAt    time.Time
		draftVersion int
		contentJSON  []byte
	)
	if err := scan(&rawID, &number, &rawTitle, &rawAuthor, &createdAt, &draftVersion, &contentJSON); err != nil {
		return nil, err
	}
	title, err := draft.NewSavedStateTitle(rawTitle)
	if err != nil {
		return nil, fmt.Errorf("stored saved state %d: %w", number, err)
	}
	author, err := draft.NewAuthor(rawAuthor)
	if err != nil {
		return nil, fmt.Errorf("stored saved state %d: %w", number, err)
	}
	var content draft.SavedStateContent
	if err := json.Unmarshal(contentJSON, &content); err != nil {
		return nil, fmt.Errorf("stored saved state %d: %w", number, err)
	}
	version, err := version.New(version.WithNumber[draft.Draft](draftVersion))
	if err != nil {
		return nil, fmt.Errorf("stored saved state %d: %w", number, err)
	}
	return draft.ReconstituteSavedState(scope, id.NewID(id.IDWithUUID[draft.SavedState](rawID)), number,
		title, author, createdAt, version, content)
}

// checkScopeLive checks that the Project or Library of scope is live: a
// deleted one does not exist for the API.
func checkScopeLive(ctx context.Context, q queryRower, scope draft.Scope) error {
	if scope.IsLibrary() {
		var live bool
		err := q.QueryRowContext(ctx,
			`SELECT deleted_at IS NULL FROM engineering.libraries WHERE id = $1`, scope.LibraryID().UUID()).Scan(&live)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
			return id.NotFound(library.ErrLibraryNotFound, scope.LibraryID())
		}
		if err != nil {
			return fmt.Errorf("cannot check the library of %s: %w", scope, err)
		}
		return nil
	}
	var live bool
	err := q.QueryRowContext(ctx,
		`SELECT deleted_at IS NULL FROM projects WHERE id = $1`, scope.ProjectID().UUID()).Scan(&live)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
		return id.NotFound(project.ErrProjectNotFound, scope.ProjectID())
	}
	if err != nil {
		return fmt.Errorf("cannot check the project of %s: %w", scope, err)
	}
	return nil
}
