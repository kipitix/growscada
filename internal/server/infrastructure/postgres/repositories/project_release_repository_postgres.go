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
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/release"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
)

// projectReleaseRepositoryPostgres stores the Project Releases in
// engineering.project_releases (task 72_3; ADR 0021). A Release is written
// with the copy of its ProjectFile and the pins that file carries, in one
// transaction over the Project.
type projectReleaseRepositoryPostgres struct {
	db       *sql.DB
	onCommit func()
}

var _ release.Repository = (*projectReleaseRepositoryPostgres)(nil)

// NewProjectReleaseRepositoryPostgres returns the repository of the Project
// Releases.
func NewProjectReleaseRepositoryPostgres(aDb *sql.DB, opts ...Option) release.Repository {
	o := newOptions(opts)
	return &projectReleaseRepositoryPostgres{db: aDb, onCommit: o.onCommit}
}

// Create publishes commit as the next Release of scope (see
// release.Repository): an advisory lock on the Project serializes concurrent
// publications, so the number is the greatest stored plus one without a retry
// loop; the unique keys are the safety net.
func (r projectReleaseRepositoryPostgres) Create(ctx context.Context, scope draft.Scope, commit draft.Commit, aNote release.Note, anAuthor draft.Author) (release.Release, error) {
	if !scope.IsProject() {
		return nil, fmt.Errorf("cannot release %s: project releases are the only ones of task 72_3", scope)
	}
	projectID := scope.ProjectID()
	var created release.Release
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		if err := lockReleaseProject(ctx, tx, projectID); err != nil {
			return nil, err
		}
		number, err := nextReleaseNumber(ctx, tx, projectID)
		if err != nil {
			return nil, err
		}
		created, err = release.CreateRelease(scope, commit, number, aNote, anAuthor, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		if err := insertProjectRelease(ctx, tx, projectID, created); err != nil {
			return nil, err
		}
		return created.PendingEvents(), nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// List returns the Releases of scope, newest first (see release.Repository).
func (r projectReleaseRepositoryPostgres) List(ctx context.Context, scope draft.Scope) ([]release.Release, error) {
	if !scope.IsProject() {
		return []release.Release{}, nil
	}
	if err := checkScopeLive(ctx, r.db, scope); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, number, commit_number, commit_title, note, author, created_at, origin, content
		 FROM engineering.project_releases WHERE project_id = $1 ORDER BY number DESC`,
		scope.ProjectID().UUID())
	if err != nil {
		return nil, fmt.Errorf("cannot read the releases of %s: %w", scope, err)
	}
	defer func() { _ = rows.Close() }()
	releases := []release.Release{}
	for rows.Next() {
		found, err := scanReleaseRow(scope, rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("cannot read the releases of %s: %w", scope, err)
		}
		releases = append(releases, found)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the releases of %s: %w", scope, err)
	}
	return releases, nil
}

// Find returns the Release number of scope (see release.Repository).
func (r projectReleaseRepositoryPostgres) Find(ctx context.Context, scope draft.Scope, number int) (release.Release, error) {
	if !scope.IsProject() {
		return nil, fmt.Errorf("%w: release %d of %s", release.ErrReleaseNotFound, number, scope)
	}
	if err := checkScopeLive(ctx, r.db, scope); err != nil {
		return nil, err
	}
	return readRelease(ctx, r.db, scope, number)
}

// UpdateNote changes the note of the Release number of scope (see
// release.Repository).
func (r projectReleaseRepositoryPostgres) UpdateNote(ctx context.Context, scope draft.Scope, number int, aNote release.Note) (release.Release, error) {
	if !scope.IsProject() {
		return nil, fmt.Errorf("%w: release %d of %s", release.ErrReleaseNotFound, number, scope)
	}
	var updated release.Release
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		if err := checkScopeLive(ctx, tx, scope); err != nil {
			return nil, err
		}
		found, err := readRelease(ctx, tx, scope, number)
		if err != nil {
			return nil, err
		}
		updated = found.WithNote(aNote)
		if _, err := tx.ExecContext(ctx,
			`UPDATE engineering.project_releases SET note = $3 WHERE project_id = $1 AND number = $2`,
			scope.ProjectID().UUID(), number, aNote.String()); err != nil {
			return nil, fmt.Errorf("cannot update the note of release %d of %s: %w", number, scope, err)
		}
		return updated.PendingEvents(), nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ReleasedCommits returns the numbers of the Commits scope published (see
// release.Repository). A Library's scope has none until task 72_5.
func (r projectReleaseRepositoryPostgres) ReleasedCommits(ctx context.Context, scope draft.Scope) ([]int, error) {
	if !scope.IsProject() {
		return []int{}, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT commit_number FROM engineering.project_releases
		 WHERE project_id = $1 AND origin = 'commit' ORDER BY number`,
		scope.ProjectID().UUID())
	if err != nil {
		return nil, fmt.Errorf("cannot read the released commits of %s: %w", scope, err)
	}
	defer func() { _ = rows.Close() }()
	numbers := []int{}
	for rows.Next() {
		var number int
		if err := rows.Scan(&number); err != nil {
			return nil, fmt.Errorf("cannot read the released commits of %s: %w", scope, err)
		}
		numbers = append(numbers, number)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the released commits of %s: %w", scope, err)
	}
	return numbers, nil
}

// inTx runs write and appends its events to the outbox in one transaction,
// signalling the dispatcher once it has committed.
func (r projectReleaseRepositoryPostgres) inTx(ctx context.Context, write func(tx *sql.Tx) ([]event.Event, error)) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	pending, err := write(tx)
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, pending); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit release change: %w", err)
	}
	if len(pending) > 0 && r.onCommit != nil {
		r.onCommit()
	}
	return nil
}

// lockReleaseProject serializes the publications of one Project: a Release
// takes its number under this advisory lock, and the Project's row is locked
// too, so a Project deleted meanwhile is not released into.
func lockReleaseProject(ctx context.Context, tx *sql.Tx, projectID id.ID[project.Project]) error {
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`,
		"project_releases:"+projectID.String()); err != nil {
		return fmt.Errorf("cannot lock the releases of project %s: %w", projectID, err)
	}
	var live bool
	err := tx.QueryRowContext(ctx,
		`SELECT deleted_at IS NULL FROM projects WHERE id = $1 FOR SHARE`, projectID.UUID()).Scan(&live)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
		return id.NotFound(project.ErrProjectNotFound, projectID)
	}
	if err != nil {
		return fmt.Errorf("cannot lock project %s: %w", projectID, err)
	}
	return nil
}

// nextReleaseNumber returns the number after the latest Release of the
// Project, under the advisory lock of lockReleaseProject.
func nextReleaseNumber(ctx context.Context, tx *sql.Tx, projectID id.ID[project.Project]) (release.Number, error) {
	var number int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(number), 0) + 1 FROM engineering.project_releases WHERE project_id = $1`,
		projectID.UUID()).Scan(&number); err != nil {
		return release.Number{}, fmt.Errorf("cannot number the release of project %s: %w", projectID, err)
	}
	return release.NewNumber(number)
}

// insertProjectRelease writes the Release and the pins of its ProjectFile,
// mapping the one-Commit-one-Release key to ErrCommitAlreadyReleased.
func insertProjectRelease(ctx context.Context, tx *sql.Tx, projectID id.ID[project.Project], found release.Release) error {
	content, err := json.Marshal(found.Content())
	if err != nil {
		return fmt.Errorf("cannot encode the content of %s: %w", found, err)
	}
	var commitNumber any
	if found.CommitNumber() > 0 {
		commitNumber = found.CommitNumber()
	}
	var commitTitle any
	if title := found.CommitTitle().String(); title != "" {
		commitTitle = title
	}
	var note any
	if text := found.Note().String(); text != "" {
		note = text
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO engineering.project_releases
		    (id, project_id, number, commit_number, commit_title, note, author, created_at, origin, content)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)`,
		found.ID().UUID(), projectID.UUID(), found.Number().Int(), commitNumber, commitTitle, note,
		found.Author().String(), found.CreatedAt(), found.Origin().String(), content,
	); err != nil {
		if isUniqueViolation(err, constraintProjectReleasesCommit) {
			return fmt.Errorf("%w: commit %d of project %s", release.ErrCommitAlreadyReleased, found.CommitNumber(), projectID)
		}
		return fmt.Errorf("cannot insert %s: %w", found, err)
	}
	file, ok := found.Content().ProjectFile()
	if !ok {
		return nil
	}
	for _, pin := range file.LibraryPins {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.project_release_library_pins (release_id, library_id, release_number) VALUES ($1, $2, $3)`,
			found.ID().UUID(), pin.LibraryID, pin.Release); err != nil {
			return fmt.Errorf("cannot insert a library pin of %s: %w", found, err)
		}
	}
	return nil
}

// readRelease reads the Release number of scope.
func readRelease(ctx context.Context, q queryRower, scope draft.Scope, number int) (release.Release, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id, number, commit_number, commit_title, note, author, created_at, origin, content
		 FROM engineering.project_releases WHERE project_id = $1 AND number = $2`,
		scope.ProjectID().UUID(), number)
	found, err := scanReleaseRow(scope, row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %d of %s", release.ErrReleaseNotFound, number, scope)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read release %d of %s: %w", number, scope, err)
	}
	return found, nil
}

// scanReleaseRow rebuilds a Release from storeReleaseColumns.
func scanReleaseRow(scope draft.Scope, scan func(...any) error) (release.Release, error) {
	var (
		rawID        uuid.UUID
		number       int
		commitNumber sql.NullInt64
		commitTitle  sql.NullString
		note         sql.NullString
		rawAuthor    string
		createdAt    time.Time
		rawOrigin    string
		contentJSON  []byte
	)
	if err := scan(&rawID, &number, &commitNumber, &commitTitle, &note, &rawAuthor, &createdAt, &rawOrigin, &contentJSON); err != nil {
		return nil, err
	}
	releaseNumber, err := release.NewNumber(number)
	if err != nil {
		return nil, fmt.Errorf("stored release %d: %w", number, err)
	}
	title, err := draft.NewCommitTitle(commitTitle.String)
	if err != nil && commitTitle.Valid {
		return nil, fmt.Errorf("stored release %d: %w", number, err)
	}
	releaseNote, err := release.NewNote(note.String)
	if err != nil {
		return nil, fmt.Errorf("stored release %d: %w", number, err)
	}
	author, err := draft.NewAuthor(rawAuthor)
	if err != nil {
		return nil, fmt.Errorf("stored release %d: %w", number, err)
	}
	origin, err := release.NewOrigin(rawOrigin)
	if err != nil {
		return nil, fmt.Errorf("stored release %d: %w", number, err)
	}
	var content draft.CommitContent
	if err := json.Unmarshal(contentJSON, &content); err != nil {
		return nil, fmt.Errorf("stored release %d: %w", number, err)
	}
	return release.ReconstituteRelease(scope, id.NewID(id.IDWithUUID[release.Release](rawID)), releaseNumber,
		int(commitNumber.Int64), title, releaseNote, author, createdAt, origin, content)
}
