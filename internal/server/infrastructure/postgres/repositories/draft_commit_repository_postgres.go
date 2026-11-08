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

// SaveCommit fixes the Draft as a Commit (see draft.Repository, task 72_2,
// ADR 0021). The content is assembled by the caller before the transaction;
// the Draft's row lock and the Version check make the snapshot the Draft at
// expected, and a write that landed during the build is a conflict.
func (r draftRepositoryPostgres) SaveCommit(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft], title draft.CommitTitle, author draft.Author, content draft.CommitContent) (draft.Commit, error) {
	var result draft.Commit
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		row, err := lockDraftRow(ctx, tx, scope, true, true)
		if err != nil {
			return nil, err
		}
		if expected.Number() != row.version {
			return nil, draft.ConflictError{Expected: expected.Number(), Current: row.version, Scope: scope}
		}
		// A Draft that did not change since its last Save has nothing to fix.
		if row.lastCommitDraftVersion == row.version {
			return nil, fmt.Errorf("%w: %s", draft.ErrNothingToSave, scope)
		}
		number := row.lastCommitNumber + 1
		commit, err := draft.NewCommit(scope, id.NewID(id.IDWithUUID[draft.Commit](uuid.New())), number,
			title, author, time.Now().UTC(), expected, content)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(commit.Content())
		if err != nil {
			return nil, fmt.Errorf("cannot encode the content of %s: %w", commit, err)
		}
		var storedAt time.Time
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO engineering.commits (id, draft_id, number, title, author, created_at, draft_version, content)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
			 RETURNING created_at`,
			commit.ID().UUID(), row.id, number, title.String(), author.String(), commit.CreatedAt(), expected.Number(), encoded,
		).Scan(&storedAt); err != nil {
			return nil, fmt.Errorf("cannot insert the commit of %s: %w", scope, err)
		}
		if err := insertCommitPins(ctx, tx, commit); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE engineering.drafts SET last_commit_number = $2, last_commit_draft_version = $3, updated_at = now() WHERE id = $1`,
			row.id, number, expected.Number()); err != nil {
			return nil, fmt.Errorf("cannot record the last commit of %s: %w", scope, err)
		}
		// The event was recorded when the Commit was made; the returned Commit
		// carries the time as stored.
		pending := commit.PendingEvents()
		result, err = draft.ReconstituteCommit(scope, commit.ID(), number, title, author, storedAt, expected, commit.Content())
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

// insertCommitPins writes the Library Pins a Project's ProjectFile carries:
// they are the provenance of the Commit and what keeps a pinned Library from
// being deleted (task 71). A Library's Commit has no pins.
func insertCommitPins(ctx context.Context, tx *sql.Tx, commit draft.Commit) error {
	file, ok := commit.Content().ProjectFile()
	if !ok {
		return nil
	}
	for _, pin := range file.LibraryPins {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.commit_library_pins (commit_id, library_id, release_number) VALUES ($1, $2, $3)`,
			commit.ID().UUID(), pin.LibraryID, pin.Release,
		); err != nil {
			return fmt.Errorf("cannot insert a library pin of %s: %w", commit, err)
		}
	}
	return nil
}

// RenameCommit changes the title of the Commit number of the Draft (see
// draft.Repository).
func (r draftRepositoryPostgres) RenameCommit(ctx context.Context, scope draft.Scope, number int, title draft.CommitTitle) (draft.Commit, error) {
	var result draft.Commit
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		if err := checkScopeLive(ctx, tx, scope); err != nil {
			return nil, err
		}
		found, err := readCommit(ctx, tx, scope, number)
		if err != nil {
			return nil, err
		}
		renamed := found.WithTitle(title)
		if _, err := tx.ExecContext(ctx,
			`UPDATE engineering.commits SET title = $2 WHERE id = $1`, found.ID().UUID(), title.String()); err != nil {
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

// FindCommits returns the Draft's Commits, newest first (see
// draft.Repository).
func (r draftRepositoryPostgres) FindCommits(ctx context.Context, scope draft.Scope) ([]draft.Commit, error) {
	if err := checkScopeLive(ctx, r.db, scope); err != nil {
		return nil, err
	}
	column, value := scopeColumn(scope)
	rows, err := r.db.QueryContext(ctx,
		`SELECT c.id, c.number, c.title, c.author, c.created_at, c.draft_version, c.content
		 FROM engineering.commits c JOIN engineering.drafts d ON d.id = c.draft_id
		 WHERE d.`+column+` = $1 ORDER BY c.number DESC`, value)
	if err != nil {
		return nil, fmt.Errorf("cannot read the commits of %s: %w", scope, err)
	}
	defer func() { _ = rows.Close() }()
	commits := []draft.Commit{}
	for rows.Next() {
		commit, err := scanCommitRow(scope, rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("cannot read the commits of %s: %w", scope, err)
		}
		commits = append(commits, commit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the commits of %s: %w", scope, err)
	}
	return commits, nil
}

// FindLastCommit returns the Draft's newest Commit, if it has one (see
// draft.Repository). It is one row: a Discard needs no other Commit.
func (r draftRepositoryPostgres) FindLastCommit(ctx context.Context, scope draft.Scope) (draft.Commit, bool, error) {
	if err := checkScopeLive(ctx, r.db, scope); err != nil {
		return nil, false, err
	}
	column, value := scopeColumn(scope)
	row := r.db.QueryRowContext(ctx,
		`SELECT c.id, c.number, c.title, c.author, c.created_at, c.draft_version, c.content
		 FROM engineering.commits c JOIN engineering.drafts d ON d.id = c.draft_id
		 WHERE d.`+column+` = $1 ORDER BY c.number DESC LIMIT 1`, value)
	commit, err := scanCommitRow(scope, row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cannot read the last commit of %s: %w", scope, err)
	}
	return commit, true, nil
}

// FindCommit returns the Commit number of the Draft (see draft.Repository).
func (r draftRepositoryPostgres) FindCommit(ctx context.Context, scope draft.Scope, number int) (draft.Commit, error) {
	if err := checkScopeLive(ctx, r.db, scope); err != nil {
		return nil, err
	}
	return readCommit(ctx, r.db, scope, number)
}

// readCommit reads one Commit of the Draft of scope.
func readCommit(ctx context.Context, q queryRower, scope draft.Scope, number int) (draft.Commit, error) {
	column, value := scopeColumn(scope)
	row := q.QueryRowContext(ctx,
		`SELECT c.id, c.number, c.title, c.author, c.created_at, c.draft_version, c.content
		 FROM engineering.commits c JOIN engineering.drafts d ON d.id = c.draft_id
		 WHERE d.`+column+` = $1 AND c.number = $2`, value, number)
	commit, err := scanCommitRow(scope, row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %d of %s", draft.ErrCommitNotFound, number, scope)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read commit %d of %s: %w", number, scope, err)
	}
	return commit, nil
}

// scanCommitRow rebuilds a Commit from a row: id, number, title, author,
// created_at, draft_version and the encoded content.
func scanCommitRow(scope draft.Scope, scan func(...any) error) (draft.Commit, error) {
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
	title, err := draft.NewCommitTitle(rawTitle)
	if err != nil {
		return nil, fmt.Errorf("stored commit %d: %w", number, err)
	}
	author, err := draft.NewAuthor(rawAuthor)
	if err != nil {
		return nil, fmt.Errorf("stored commit %d: %w", number, err)
	}
	var content draft.CommitContent
	if err := json.Unmarshal(contentJSON, &content); err != nil {
		return nil, fmt.Errorf("stored commit %d: %w", number, err)
	}
	version, err := version.New(version.WithNumber[draft.Draft](draftVersion))
	if err != nil {
		return nil, fmt.Errorf("stored commit %d: %w", number, err)
	}
	return draft.ReconstituteCommit(scope, id.NewID(id.IDWithUUID[draft.Commit](rawID)), number,
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
