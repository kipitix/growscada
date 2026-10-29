package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

type libraryRepositoryPostgresImpl struct {
	db    *sql.DB
	store aggregateStore[library.Library]
}

var _ library.LibraryRepository = (*libraryRepositoryPostgresImpl)(nil)

func NewLibraryRepositoryPostgres(aDb *sql.DB, opts ...Option) library.LibraryRepository {
	o := newOptions(opts)
	return &libraryRepositoryPostgresImpl{
		db: aDb,
		store: aggregateStore[library.Library]{
			db: aDb, table: tableNameLibraries,
			notFound: library.ErrLibraryNotFound, conflict: library.ErrLibraryConflict,
			onCommit: o.onCommit,
		},
	}
}

func (r libraryRepositoryPostgresImpl) NextID() id.ID[library.Library] {
	return id.NewID[library.Library]()
}

// Save writes the libraries row. The CAS on update matches only a live row,
// and a row it does not match is classified by whether a live one is there.
// A library marked deleted is checked for pins after its row is updated:
// the update waits for a pin being added (which locks the row for share) and
// a pin added later finds the row deleted.
func (r libraryRepositoryPostgresImpl) Save(ctx context.Context, l library.Library) (library.Library, error) {
	discardingTo := nullableRelease(l.DiscardingTo())
	saved, err := r.store.save(ctx, l.ID(), l.Version(), l.PendingEvents(), rowWrite[library.Library]{
		insert: func(tx *sql.Tx, next version.Version[library.Library]) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO engineering.libraries (id, organization_id, name, discarding_to, version, deleted_at)
				 VALUES ($1, $2, $3, $4, $5, CASE WHEN $6 THEN now() END)`,
				l.ID().UUID(), l.OrganizationID().UUID(), l.Name().String(), discardingTo, next.Number(), l.IsDeleted(),
			)
			return libraryWriteError("insert new library", err)
		},
		update: func(tx *sql.Tx, current, next version.Version[library.Library]) (bool, error) {
			res, err := tx.ExecContext(ctx,
				`UPDATE engineering.libraries
				 SET name = $1, discarding_to = $2, version = $3, deleted_at = CASE WHEN $4 THEN now() END
				 WHERE id = $5 AND version = $6 AND deleted_at IS NULL`,
				l.Name().String(), discardingTo, next.Number(), l.IsDeleted(), l.ID().UUID(), current.Number(),
			)
			if err != nil {
				return false, libraryWriteError("update library", err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return false, fmt.Errorf("cannot get rows affected on update: %w", err)
			}
			if n == 1 {
				if l.IsDeleted() {
					return true, r.checkNotPinned(ctx, tx, l.ID())
				}
				if release, discarding := l.DiscardingTo(); discarding {
					return true, r.checkLatestRelease(ctx, tx, l.ID(), release)
				}
				return true, nil
			}
			// A library marked deleted is gone for the CAS: not found, not a
			// conflict, whatever its version.
			var live bool
			err = tx.QueryRowContext(ctx, `SELECT deleted_at IS NULL FROM engineering.libraries WHERE id = $1`, l.ID().UUID()).Scan(&live)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && !live) {
				return false, id.NotFound(library.ErrLibraryNotFound, l.ID())
			}
			if err != nil {
				return false, fmt.Errorf("cannot check library existence: %w", err)
			}
			return false, nil
		},
	})
	if err != nil {
		return nil, err
	}
	release, _ := l.DiscardingTo()
	return library.ReconstituteLibrary(l.ID(), l.OrganizationID(), l.Name(), l.IsDeleted(), release, saved), nil
}

// checkLatestRelease fails with ErrLibraryConflict unless release is the
// latest of the library. The row is locked for update by then: a Release
// committed before is seen, one after finds the Discard's mark (ADR 0012).
func (r libraryRepositoryPostgresImpl) checkLatestRelease(ctx context.Context, tx *sql.Tx, anID id.ID[library.Library], release library.ReleaseNumber) error {
	var latest int
	err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(max(number), 0) FROM engineering.library_releases WHERE library_id = $1`, anID.UUID(),
	).Scan(&latest)
	if err != nil {
		return fmt.Errorf("cannot find latest release of library %s: %w", anID, err)
	}
	if latest != release.Int() {
		return fmt.Errorf("%w: discarding to %s, latest release %d", library.ErrLibraryConflict, release, latest)
	}
	return nil
}

// checkNotPinned fails with ErrLibraryPinned if a live Project pins the
// library.
func (r libraryRepositoryPostgresImpl) checkNotPinned(ctx context.Context, tx *sql.Tx, anID id.ID[library.Library]) error {
	var pinned bool
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM engineering.library_pins lp JOIN projects p ON p.id = lp.project_id
		                WHERE lp.library_id = $1 AND p.deleted_at IS NULL)`,
		anID.UUID(),
	).Scan(&pinned)
	if err != nil {
		return fmt.Errorf("cannot check pins of library %s: %w", anID, err)
	}
	if pinned {
		return fmt.Errorf("%w: %s", library.ErrLibraryPinned, anID)
	}
	return nil
}

// nullableRelease is the discarding_to column of a Discard's mark.
func nullableRelease(n library.ReleaseNumber, ok bool) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(n.Int()), Valid: ok}
}

// libraryWriteError maps a failed write of a libraries row: a name taken by a
// live library of the organization is ErrLibraryNameTaken.
func libraryWriteError(what string, err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err, constraintLibrariesLiveName) {
		return library.ErrLibraryNameTaken
	}
	return fmt.Errorf("cannot %s: %w", what, err)
}

// Delete removes the row of a library marked deleted, with its Draft and its
// releases by the cascade of their foreign keys. The pins of Projects still
// being removed refuse it.
func (r libraryRepositoryPostgresImpl) Delete(ctx context.Context, l library.Library) error {
	return r.store.inTx(ctx, l.PendingEvents(), func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM engineering.libraries WHERE id = $1 AND deleted_at IS NOT NULL`, l.ID().UUID())
		if isForeignKeyViolation(err, constraintLibraryPinsLibraryID) || isForeignKeyViolation(err, constraintLibraryPinsRelease) {
			return fmt.Errorf("%w: %s", library.ErrLibraryPinned, l.ID())
		}
		if err != nil {
			return fmt.Errorf("cannot delete from libraries: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on delete: %w", err)
		}
		if n != 1 {
			return id.NotFound(library.ErrLibraryNotFound, l.ID())
		}
		return nil
	})
}

const selectLibraryColumns = `id, organization_id, name, deleted_at IS NOT NULL, discarding_to, version`

func (r libraryRepositoryPostgresImpl) FindByID(ctx context.Context, anID id.ID[library.Library]) (library.Library, error) {
	return r.findOne(ctx, anID, `WHERE id = $1 AND deleted_at IS NULL`, anID.UUID())
}

func (r libraryRepositoryPostgresImpl) FindDeleted(ctx context.Context, anID id.ID[library.Library]) (library.Library, error) {
	return r.findOne(ctx, anID, `WHERE id = $1 AND deleted_at IS NOT NULL`, anID.UUID())
}

func (r libraryRepositoryPostgresImpl) findOne(ctx context.Context, anID id.ID[library.Library], where string, args ...any) (library.Library, error) {
	found, err := scanLibrary(r.db.QueryRowContext(ctx, `SELECT `+selectLibraryColumns+` FROM engineering.libraries `+where, args...).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, id.NotFound(library.ErrLibraryNotFound, anID)
	}
	if err != nil {
		return nil, fmt.Errorf("error scanning library: %w", err)
	}
	return found, nil
}

func (r libraryRepositoryPostgresImpl) FindByName(ctx context.Context, orgID id.ID[organization.Organization], aName library.LibraryName) (library.Library, error) {
	found, err := scanLibrary(r.db.QueryRowContext(ctx,
		`SELECT `+selectLibraryColumns+` FROM engineering.libraries
		 WHERE organization_id = $1 AND name = $2 AND deleted_at IS NULL`,
		orgID.UUID(), aName.String(),
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, library.ErrLibraryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error scanning library: %w", err)
	}
	return found, nil
}

func (r libraryRepositoryPostgresImpl) FindAll(ctx context.Context, orgID id.ID[organization.Organization]) ([]library.Library, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectLibraryColumns+` FROM engineering.libraries
		 WHERE organization_id = $1 AND deleted_at IS NULL ORDER BY pk_id`,
		orgID.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying libraries: %w", err)
	}
	defer rows.Close()

	libraries := []library.Library{}
	for rows.Next() {
		l, err := scanLibrary(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning library: %w", err)
		}
		libraries = append(libraries, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over libraries: %w", err)
	}
	return libraries, nil
}

// scanLibrary reads a row of selectLibraryColumns.
func scanLibrary(scan func(...any) error) (library.Library, error) {
	var (
		rawID, rawOrgID uuid.UUID
		rawName         string
		deleted         bool
		discardingTo    sql.NullInt64
		rawVersion      int
	)
	if err := scan(&rawID, &rawOrgID, &rawName, &deleted, &discardingTo, &rawVersion); err != nil {
		return nil, err
	}
	name, err := library.NewLibraryName(rawName)
	if err != nil {
		return nil, fmt.Errorf("cannot create library name: %w", err)
	}
	var release library.ReleaseNumber
	if discardingTo.Valid {
		if release, err = library.NewReleaseNumber(int(discardingTo.Int64)); err != nil {
			return nil, fmt.Errorf("cannot create release number: %w", err)
		}
	}
	ver, err := version.New(version.WithNumber[library.Library](rawVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create library version: %w", err)
	}
	return library.ReconstituteLibrary(
		id.NewID(id.IDWithUUID[library.Library](rawID)),
		id.NewID(id.IDWithUUID[organization.Organization](rawOrgID)),
		name, deleted, release, ver,
	), nil
}

// lockedLibrary is the row of a live Library as read under its lock.
type lockedLibrary struct {
	discardingTo sql.NullInt64
	version      int
}

// lockLibraryRow locks the row of the live Library libraryID for share in
// tx and returns it; one marked deleted (or removed) is not found. Every
// write that depends on a Library's state takes it — a WidgetType of its
// Draft, a new pin —, so a write of the row itself (a Discard's mark, a
// rename, the mark of its deletion) waits for them, or they for it (ADR 0012).
func lockLibraryRow(ctx context.Context, tx *sql.Tx, libraryID id.ID[library.Library]) (lockedLibrary, error) {
	return lockLibraryRowFor(ctx, tx, libraryID, `FOR SHARE`)
}

// lockLibraryRowExclusive is lockLibraryRow for update: a release takes it so
// that the writes of the Draft, which hold it for share, wait for the release
// or it for them, and the Draft it reads is the one it copies.
func lockLibraryRowExclusive(ctx context.Context, tx *sql.Tx, libraryID id.ID[library.Library]) (lockedLibrary, error) {
	return lockLibraryRowFor(ctx, tx, libraryID, `FOR UPDATE`)
}

func lockLibraryRowFor(ctx context.Context, tx *sql.Tx, libraryID id.ID[library.Library], lock string) (lockedLibrary, error) {
	var row lockedLibrary
	err := tx.QueryRowContext(ctx,
		`SELECT discarding_to, version FROM engineering.libraries WHERE id = $1 AND deleted_at IS NULL `+lock, libraryID.UUID(),
	).Scan(&row.discardingTo, &row.version)
	if errors.Is(err, sql.ErrNoRows) {
		return row, id.NotFound(library.ErrLibraryNotFound, libraryID)
	}
	if err != nil {
		return row, fmt.Errorf("cannot lock library %s: %w", libraryID, err)
	}
	return row, nil
}
