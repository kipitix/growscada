package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
)

type libraryReleaseRepositoryPostgresImpl struct {
	db       *sql.DB
	onCommit func()
}

var _ library.LibraryReleaseRepository = (*libraryReleaseRepositoryPostgresImpl)(nil)

func NewLibraryReleaseRepositoryPostgres(aDb *sql.DB, opts ...Option) library.LibraryReleaseRepository {
	o := newOptions(opts)
	return &libraryReleaseRepositoryPostgresImpl{db: aDb, onCommit: o.onCommit}
}

// Save inserts the release with the copies of its WidgetTypes, in one
// transaction over its Library. The unique keys make a release that lost the
// race for its number, or a second Release of the same Commit, lose
// (task 72_5; ADR 0021). A Release made from a Commit stores its number, its
// Author and its origin; a legacy one names no Commit.
func (r libraryReleaseRepositoryPostgresImpl) Save(ctx context.Context, release library.LibraryRelease) (library.LibraryRelease, error) {
	libraryID := release.LibraryID()
	var createdAt time.Time // as stored, to the column's precision
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		if err := lockLiveLibrary(ctx, tx, libraryID); err != nil {
			return nil, err
		}
		var commitNumber sql.NullInt64
		if number := release.CommitNumber(); number != nil {
			commitNumber = sql.NullInt64{Int64: int64(*number), Valid: true}
		}
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO engineering.library_releases
			    (library_id, number, commit_number, author, origin, note, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING created_at`,
			libraryID.UUID(), release.Number().Int(), commitNumber, release.Author().String(),
			release.Origin().String(), release.Note(), release.CreatedAt(),
		).Scan(&createdAt); err != nil {
			switch {
			case isUniqueViolation(err, constraintReleasesNumber):
				return nil, fmt.Errorf("%w: %s of library %s", library.ErrReleaseNumberTaken, release.Number(), libraryID)
			case isUniqueViolation(err, constraintLibraryReleasesCommit):
				return nil, fmt.Errorf("%w: %v of library %s", library.ErrCommitAlreadyReleased, release.CommitNumber(), libraryID)
			}
			return nil, fmt.Errorf("cannot insert release: %w", err)
		}
		for _, wt := range release.WidgetTypes() {
			portsJSON, err := marshalInputPorts(wt.InputPorts())
			if err != nil {
				return nil, fmt.Errorf("cannot serialize input ports: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO engineering.released_widget_types
				    (library_id, release_number, id, name, html_template, script, script_language, default_width, default_height, input_ports)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				libraryID.UUID(), release.Number().Int(), wt.ID().UUID(), wt.Name().String(), wt.HtmlTemplate().String(),
				wt.Script().String(), wt.ScriptLanguage().String(), wt.DefaultSize().Width(), wt.DefaultSize().Height(), portsJSON,
			); err != nil {
				return nil, fmt.Errorf("cannot insert released widget type %s: %w", wt.ID(), err)
			}
		}
		return release.PendingEvents(), nil
	})
	if err != nil {
		return nil, err
	}
	saved, err := library.ReconstituteRelease(libraryID, release.Number(), release.CommitNumber(), release.Author(),
		release.Origin(), release.Note(), createdAt, release.WidgetTypes())
	if err != nil {
		return nil, fmt.Errorf("cannot rebuild saved release: %w", err)
	}
	return saved, nil
}

func (r libraryReleaseRepositoryPostgresImpl) FindLatest(ctx context.Context, libraryID id.ID[library.Library]) (library.LibraryRelease, error) {
	found, err := r.readReleases(ctx, libraryID, `AND number = (SELECT max(number) FROM engineering.library_releases WHERE library_id = $1)`)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	return found[0], nil
}

func (r libraryReleaseRepositoryPostgresImpl) FindByNumber(ctx context.Context, libraryID id.ID[library.Library], number library.ReleaseNumber) (library.LibraryRelease, error) {
	found, err := r.readReleases(ctx, libraryID, `AND number = $2`, number.Int())
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("%w: %s of library %s", library.ErrReleaseNotFound, number, libraryID)
	}
	return found[0], nil
}

func (r libraryReleaseRepositoryPostgresImpl) FindAll(ctx context.Context, libraryID id.ID[library.Library]) ([]library.LibraryRelease, error) {
	return r.readReleases(ctx, libraryID, ``)
}

// UpdateNote changes the note of the release of the Library and records
// library_release_note_updated, in one transaction.
func (r libraryReleaseRepositoryPostgresImpl) UpdateNote(ctx context.Context, libraryID id.ID[library.Library], number library.ReleaseNumber, note string) (library.LibraryRelease, error) {
	var updated library.LibraryRelease
	err := r.inTx(ctx, func(tx *sql.Tx) ([]event.Event, error) {
		if err := lockLiveLibrary(ctx, tx, libraryID); err != nil {
			return nil, err
		}
		found, err := readLibraryRelease(ctx, tx, libraryID, number)
		if err != nil {
			return nil, err
		}
		types, err := readReleaseWidgetTypes(ctx, tx, libraryID, number)
		if err != nil {
			return nil, err
		}
		found, err = library.ReconstituteRelease(found.LibraryID(), found.Number(), found.CommitNumber(),
			found.Author(), found.Origin(), found.Note(), found.CreatedAt(), types)
		if err != nil {
			return nil, fmt.Errorf("cannot rebuild release %s of library %s: %w", number, libraryID, err)
		}
		updated = found.WithNote(note)
		if _, err := tx.ExecContext(ctx,
			`UPDATE engineering.library_releases SET note = $3 WHERE library_id = $1 AND number = $2`,
			libraryID.UUID(), number.Int(), note); err != nil {
			return nil, fmt.Errorf("cannot update the note of release %s of library %s: %w", number, libraryID, err)
		}
		return updated.PendingEvents(), nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// readReleaseWidgetTypes reads the copies of the WidgetTypes of a Release in q.
func readReleaseWidgetTypes(ctx context.Context, tx *sql.Tx, libraryID id.ID[library.Library], number library.ReleaseNumber) ([]library.ReleasedWidgetType, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT `+selectReleasedWidgetTypeColumns+` FROM engineering.released_widget_types
		 WHERE library_id = $1 AND release_number = $2 ORDER BY pk_id`,
		libraryID.UUID(), number.Int())
	if err != nil {
		return nil, fmt.Errorf("cannot read the widget types of release %s: %w", number, err)
	}
	defer func() { _ = rows.Close() }()
	types := []library.ReleasedWidgetType{}
	for rows.Next() {
		wt, err := scanReleasedWidgetType(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("cannot read the widget types of release %s: %w", number, err)
		}
		types = append(types, wt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the widget types of release %s: %w", number, err)
	}
	return types, nil
}

// ReleasedCommits returns the numbers of the Commits the Library published as
// Releases, oldest first.
func (r libraryReleaseRepositoryPostgresImpl) ReleasedCommits(ctx context.Context, libraryID id.ID[library.Library]) ([]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT commit_number FROM engineering.library_releases
		 WHERE library_id = $1 AND origin = 'commit' ORDER BY number`,
		libraryID.UUID())
	if err != nil {
		return nil, fmt.Errorf("cannot read the released commits of library %s: %w", libraryID, err)
	}
	defer func() { _ = rows.Close() }()
	numbers := []int{}
	for rows.Next() {
		var number int
		if err := rows.Scan(&number); err != nil {
			return nil, fmt.Errorf("cannot read the released commits of library %s: %w", libraryID, err)
		}
		numbers = append(numbers, number)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the released commits of library %s: %w", libraryID, err)
	}
	return numbers, nil
}

// inTx runs write and appends its events to the outbox in one transaction,
// signalling the dispatcher once it has committed.
func (r libraryReleaseRepositoryPostgresImpl) inTx(ctx context.Context, write func(tx *sql.Tx) ([]event.Event, error)) error {
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

// lockLiveLibrary locks the live Library's row FOR SHARE, so it cannot be
// marked deleted while its release is written. A Library that is not live is
// ErrLibraryNotFound.
func lockLiveLibrary(ctx context.Context, tx *sql.Tx, libraryID id.ID[library.Library]) error {
	var live bool
	err := tx.QueryRowContext(ctx,
		`SELECT deleted_at IS NULL FROM engineering.libraries WHERE id = $1 FOR SHARE`,
		libraryID.UUID()).Scan(&live)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
		return id.NotFound(library.ErrLibraryNotFound, libraryID)
	}
	if err != nil {
		return fmt.Errorf("cannot lock library %s: %w", libraryID, err)
	}
	return nil
}

// readLibraryRelease reads the release number of the Library in q.
func readLibraryRelease(ctx context.Context, q queryRower, libraryID id.ID[library.Library], number library.ReleaseNumber) (library.LibraryRelease, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+selectLibraryReleaseColumns+` FROM engineering.library_releases WHERE library_id = $1 AND number = $2`,
		libraryID.UUID(), number.Int())
	found, err := scanLibraryRelease(libraryID, number.Int(), row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s of library %s", library.ErrReleaseNotFound, number, libraryID)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read release %s of library %s: %w", number, libraryID, err)
	}
	return found, nil
}

// rawLibraryRelease is a library_releases row without its WidgetTypes, before
// the copies are read and attached.
type rawLibraryRelease struct {
	number       int
	commitNumber sql.NullInt64
	author       string
	origin       string
	note         string
	createdAt    time.Time
}

// readReleases reads the releases of the Library matching and, a further
// condition whose arguments are numbered from $2, oldest first, with their
// WidgetTypes, in one read-only transaction.
func (r libraryReleaseRepositoryPostgresImpl) readReleases(ctx context.Context, libraryID id.ID[library.Library], and string, args ...any) ([]library.LibraryRelease, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT number, `+selectLibraryReleaseColumns+` FROM engineering.library_releases WHERE library_id = $1 `+and+` ORDER BY number`,
		append([]any{libraryID.UUID()}, args...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying releases: %w", err)
	}
	var raws []rawLibraryRelease
	for rows.Next() {
		var raw rawLibraryRelease
		if err := rows.Scan(&raw.number, &raw.commitNumber, &raw.author, &raw.origin, &raw.note, &raw.createdAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("error scanning release: %w", err)
		}
		raws = append(raws, raw)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over releases: %w", err)
	}
	if len(raws) == 0 {
		return []library.LibraryRelease{}, nil
	}

	numbers := make([]int64, len(raws))
	for i, raw := range raws {
		numbers[i] = int64(raw.number)
	}
	typeRows, err := tx.QueryContext(ctx,
		`SELECT release_number, `+selectReleasedWidgetTypeColumns+` FROM engineering.released_widget_types
		 WHERE library_id = $1 AND release_number = ANY($2) ORDER BY pk_id`,
		libraryID.UUID(), pq.Array(numbers),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying released widget types: %w", err)
	}
	defer typeRows.Close()
	typesByRelease := map[int][]library.ReleasedWidgetType{}
	for typeRows.Next() {
		var number int
		wt, err := scanReleasedWidgetType(func(dest ...any) error { return typeRows.Scan(append([]any{&number}, dest...)...) })
		if err != nil {
			return nil, fmt.Errorf("error scanning released widget type: %w", err)
		}
		typesByRelease[number] = append(typesByRelease[number], wt)
	}
	if err := typeRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over released widget types: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot commit release read: %w", err)
	}

	releases := make([]library.LibraryRelease, len(raws))
	for i, raw := range raws {
		rebuilt, err := rebuildLibraryRelease(libraryID, raw, typesByRelease[raw.number])
		if err != nil {
			return nil, err
		}
		releases[i] = rebuilt
	}
	return releases, nil
}

// rebuildLibraryRelease builds the release of a row and its copies.
func rebuildLibraryRelease(libraryID id.ID[library.Library], raw rawLibraryRelease, types []library.ReleasedWidgetType) (library.LibraryRelease, error) {
	number, err := library.NewReleaseNumber(raw.number)
	if err != nil {
		return nil, fmt.Errorf("stored release %d: %w", raw.number, err)
	}
	author, err := library.NewAuthor(raw.author)
	if err != nil {
		return nil, fmt.Errorf("stored release %d: %w", raw.number, err)
	}
	origin, err := library.NewOrigin(raw.origin)
	if err != nil {
		return nil, fmt.Errorf("stored release %d: %w", raw.number, err)
	}
	var commit *int
	if raw.commitNumber.Valid {
		n := int(raw.commitNumber.Int64)
		commit = &n
	}
	rebuilt, err := library.ReconstituteRelease(libraryID, number, commit, author, origin, raw.note, raw.createdAt, types)
	if err != nil {
		return nil, fmt.Errorf("cannot rebuild release %d: %w", raw.number, err)
	}
	return rebuilt, nil
}

// selectLibraryReleaseColumns lists the library_releases columns after
// number, in the order scanLibraryRelease reads them.
const selectLibraryReleaseColumns = `commit_number, author, origin, note, created_at`

// scanLibraryRelease reads a row of selectLibraryReleaseColumns (its number
// is known by the caller).
func scanLibraryRelease(libraryID id.ID[library.Library], number int, scan func(...any) error) (library.LibraryRelease, error) {
	var raw rawLibraryRelease
	raw.number = number
	if err := scan(&raw.commitNumber, &raw.author, &raw.origin, &raw.note, &raw.createdAt); err != nil {
		return nil, err
	}
	return rebuildLibraryRelease(libraryID, raw, nil)
}

// constraintLibraryReleasesCommit is the partial unique index on a Library
// Release's Commit: one Commit is published at most once (task 72_5).
const constraintLibraryReleasesCommit = "uq_library_releases_commit"

// selectReleasedWidgetTypeColumns lists the released_widget_types columns of
// scanReleasedWidgetType, in its order.
const selectReleasedWidgetTypeColumns = `id, library_id, name, html_template, script, script_language, default_width, default_height, input_ports`

// scanReleasedWidgetType reads a row of selectReleasedWidgetTypeColumns.
func scanReleasedWidgetType(scan func(...any) error) (library.ReleasedWidgetType, error) {
	var (
		rawID, rawLibraryID   uuid.UUID
		rawName, html, script string
		language              string
		width, height         int
		portsJSON             []byte
	)
	if err := scan(&rawID, &rawLibraryID, &rawName, &html, &script, &language, &width, &height, &portsJSON); err != nil {
		return library.ReleasedWidgetType{}, err
	}
	name, err := library.NewWidgetTypeName(rawName)
	if err != nil {
		return library.ReleasedWidgetType{}, fmt.Errorf("cannot create widget type name: %w", err)
	}
	htmlTemplate, err := library.NewHtmlTemplate(html)
	if err != nil {
		return library.ReleasedWidgetType{}, fmt.Errorf("cannot create html template: %w", err)
	}
	newScript, err := library.NewScript(script)
	if err != nil {
		return library.ReleasedWidgetType{}, fmt.Errorf("cannot create script: %w", err)
	}
	lang, err := library.NewScriptLanguage(language)
	if err != nil {
		return library.ReleasedWidgetType{}, fmt.Errorf("cannot create script language: %w", err)
	}
	size, err := library.NewSize(width, height)
	if err != nil {
		return library.ReleasedWidgetType{}, fmt.Errorf("cannot create default size: %w", err)
	}
	ports, err := unmarshalInputPorts(portsJSON)
	if err != nil {
		return library.ReleasedWidgetType{}, fmt.Errorf("cannot unmarshal input ports: %w", err)
	}
	return library.NewReleasedWidgetType(
		id.NewID(id.IDWithUUID[library.WidgetType](rawID)),
		id.NewID(id.IDWithUUID[library.Library](rawLibraryID)),
		name, htmlTemplate, newScript, lang, size, ports,
	)
}
