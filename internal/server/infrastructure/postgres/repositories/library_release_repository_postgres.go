package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

type libraryReleaseRepositoryPostgresImpl struct {
	db    *sql.DB
	store aggregateStore[library.LibraryRelease]
}

var _ library.LibraryReleaseRepository = (*libraryReleaseRepositoryPostgresImpl)(nil)

func NewLibraryReleaseRepositoryPostgres(aDb *sql.DB, opts ...Option) library.LibraryReleaseRepository {
	o := newOptions(opts)
	return &libraryReleaseRepositoryPostgresImpl{
		db:    aDb,
		store: aggregateStore[library.LibraryRelease]{db: aDb, table: tableNameReleases, onCommit: o.onCommit},
	}
}

// Save inserts the release with the copies of its WidgetTypes. It locks the
// Library's row for share, as a write of a WidgetType does, and checks its
// version there: a Discard marked or a rename made meanwhile waits for it,
// or it for them and is refused (ADR 0012).
func (r libraryReleaseRepositoryPostgresImpl) Save(ctx context.Context, release library.LibraryRelease, expected version.Version[library.Library]) (library.LibraryRelease, error) {
	libraryID := release.LibraryID()
	err := r.store.inTx(ctx, release.PendingEvents(), func(tx *sql.Tx) error {
		row, err := lockLibraryRow(ctx, tx, libraryID)
		switch {
		case err != nil:
			return err
		case row.version != expected.Number():
			return fmt.Errorf("%w: expected version %s, current %d", library.ErrLibraryConflict, expected, row.version)
		case row.discardingTo.Valid:
			return fmt.Errorf("%w: %s", library.ErrLibraryDiscarding, libraryID)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO engineering.library_releases (library_id, number, note, created_at) VALUES ($1, $2, $3, $4)`,
			libraryID.UUID(), release.Number().Int(), release.Note(), release.CreatedAt(),
		); err != nil {
			if isUniqueViolation(err, constraintReleasesNumber) {
				return fmt.Errorf("%w: %s of library %s", library.ErrReleaseNumberTaken, release.Number(), libraryID)
			}
			return fmt.Errorf("cannot insert release: %w", err)
		}
		for _, wt := range release.WidgetTypes() {
			portsJSON, err := marshalInputPorts(wt.InputPorts())
			if err != nil {
				return fmt.Errorf("cannot serialize input ports: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO engineering.released_widget_types
				    (library_id, release_number, id, name, html_template, script, script_language, default_width, default_height, input_ports)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				libraryID.UUID(), release.Number().Int(), wt.ID().UUID(), wt.Name().String(), wt.HtmlTemplate().String(),
				wt.Script().String(), wt.ScriptLanguage().String(), wt.DefaultSize().Width(), wt.DefaultSize().Height(), portsJSON,
			); err != nil {
				return fmt.Errorf("cannot insert released widget type %s: %w", wt.ID(), err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return library.ReconstituteRelease(libraryID, release.Number(), release.Note(), release.CreatedAt(), release.WidgetTypes()), nil
}

func (r libraryReleaseRepositoryPostgresImpl) FindLatest(ctx context.Context, libraryID id.ID[library.Library]) (library.LibraryRelease, error) {
	found, err := r.readReleases(ctx, libraryID, `AND number = (SELECT max(number) FROM engineering.library_releases WHERE library_id = $1)`)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	return found[0], nil
}

// releaseStatesQuery computes the ReleaseState of each Library of $1 in
// one statement: a Draft differs from the latest release when a WidgetType
// is in one but not the other, or any column library.ReleasedWidgetType.Matches
// compares differs.
const releaseStatesQuery = `SELECT l.id, COALESCE(lr.latest, 0), lr.latest IS NULL OR EXISTS (
		SELECT 1
		FROM (SELECT * FROM engineering.widget_types WHERE library_id = l.id) d
		FULL JOIN (SELECT * FROM engineering.released_widget_types WHERE library_id = l.id AND release_number = lr.latest) x
			ON x.id = d.id
		WHERE d.id IS NULL OR x.id IS NULL
		   OR (d.name, d.html_template, d.script, d.script_language, d.default_width, d.default_height, d.input_ports)
		      IS DISTINCT FROM (x.name, x.html_template, x.script, x.script_language, x.default_width, x.default_height, x.input_ports))
	FROM unnest($1::uuid[]) AS l(id)
	LEFT JOIN LATERAL (SELECT max(number) AS latest FROM engineering.library_releases WHERE library_id = l.id) lr ON true`

func (r libraryReleaseRepositoryPostgresImpl) FindReleaseStates(ctx context.Context, libraryIDs []id.ID[library.Library]) (map[id.ID[library.Library]]library.ReleaseState, error) {
	raw := make([]string, len(libraryIDs))
	for i, libraryID := range libraryIDs {
		raw[i] = libraryID.UUID().String()
	}
	rows, err := r.db.QueryContext(ctx, releaseStatesQuery, pq.Array(raw))
	if err != nil {
		return nil, fmt.Errorf("error querying release states: %w", err)
	}
	defer rows.Close()
	states := make(map[id.ID[library.Library]]library.ReleaseState, len(libraryIDs))
	for rows.Next() {
		var libraryUUID uuid.UUID
		var latest int
		var state library.ReleaseState
		if err := rows.Scan(&libraryUUID, &latest, &state.DraftDiffers); err != nil {
			return nil, fmt.Errorf("error scanning release state: %w", err)
		}
		if latest > 0 {
			if state.Latest, err = library.NewReleaseNumber(latest); err != nil {
				return nil, err
			}
		}
		states[id.NewID(id.IDWithUUID[library.Library](libraryUUID))] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over release states: %w", err)
	}
	return states, nil
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

// readReleases reads the releases of the Library matching and, a further
// condition whose arguments are numbered from $2, oldest first, with their
// WidgetTypes, in one read-only transaction.
func (r libraryReleaseRepositoryPostgresImpl) readReleases(ctx context.Context, libraryID id.ID[library.Library], and string, args ...any) ([]library.LibraryRelease, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	type rawRelease struct {
		number    int
		note      string
		createdAt time.Time
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT number, note, created_at FROM engineering.library_releases WHERE library_id = $1 `+and+` ORDER BY number`,
		append([]any{libraryID.UUID()}, args...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying releases: %w", err)
	}
	var raws []rawRelease
	for rows.Next() {
		var raw rawRelease
		if err := rows.Scan(&raw.number, &raw.note, &raw.createdAt); err != nil {
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
		number, err := library.NewReleaseNumber(raw.number)
		if err != nil {
			return nil, fmt.Errorf("cannot create release number: %w", err)
		}
		releases[i] = library.ReconstituteRelease(libraryID, number, raw.note, raw.createdAt, typesByRelease[raw.number])
	}
	return releases, nil
}

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
