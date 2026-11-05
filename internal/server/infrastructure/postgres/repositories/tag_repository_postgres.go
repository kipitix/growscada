package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// tagRepositoryPostgresImpl reads the Tag definitions of one Project
// (engineering.tags, ADR 0011): every query names project_id. They are
// entities of the Project's Draft (ADR 0016): their writes go through
// draftRepositoryPostgres.
type tagRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
}

var _ tag.TagRepository = (*tagRepositoryPostgresImpl)(nil)

// NewTagRepositoryPostgres returns the repository of the tags of the Project
// aProjectID (ProjectScopes.ForProject is how the server opens one).
func NewTagRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], _ ...Option) tag.TagRepository {
	return &tagRepositoryPostgresImpl{db: aDb, project: aProjectID}
}

func (r tagRepositoryPostgresImpl) NextID() id.ID[tag.Tag] {
	return id.NewID[tag.Tag]()
}

func (r tagRepositoryPostgresImpl) FindByID(ctx context.Context, tagID id.ID[tag.Tag]) (tag.Tag, error) {
	found, err := r.scanTag(r.db.QueryRowContext(ctx,
		`SELECT id, name, type, device_id FROM engineering.tags WHERE id = $1 AND project_id = $2`,
		tagID.UUID(), r.project.UUID(),
	))
	if errors.Is(err, tag.ErrTagNotFound) {
		return nil, id.NotFound(tag.ErrTagNotFound, tagID)
	}
	return found, err
}

func (r tagRepositoryPostgresImpl) FindTypesByIDs(ctx context.Context, tagIDs []id.ID[tag.Tag]) (map[id.ID[tag.Tag]]tag.TagType, error) {
	types := make(map[id.ID[tag.Tag]]tag.TagType, len(tagIDs))
	if len(tagIDs) == 0 {
		return types, nil
	}
	rawIDs := make([]string, len(tagIDs))
	for i, tagID := range tagIDs {
		rawIDs[i] = tagID.UUID().String()
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, type FROM engineering.tags WHERE id = ANY($1::uuid[]) AND project_id = $2`,
		pq.Array(rawIDs), r.project.UUID())
	if err != nil {
		return nil, fmt.Errorf("error querying tag types: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			tagUUID uuid.UUID
			rawType string
		)
		if err := rows.Scan(&tagUUID, &rawType); err != nil {
			return nil, fmt.Errorf("error scanning tag type: %w", err)
		}
		tagType, err := tag.NewTagType(rawType)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag type: %w", err)
		}
		types[id.NewID(id.IDWithUUID[tag.Tag](tagUUID))] = tagType
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over tag types: %w", err)
	}
	return types, nil
}

func (r tagRepositoryPostgresImpl) FindByName(ctx context.Context, aName tag.TagName) (tag.Tag, error) {
	return r.scanTag(r.db.QueryRowContext(ctx,
		`SELECT id, name, type, device_id FROM engineering.tags WHERE name = $1 AND project_id = $2`,
		aName.String(), r.project.UUID(),
	))
}

// scanTag reads a single `id, name, type, device_id` row; no row is
// tag.ErrTagNotFound.
func (r tagRepositoryPostgresImpl) scanTag(row *sql.Row) (tag.Tag, error) {
	var (
		tagUUID  uuid.UUID
		name     string
		tagType  string
		deviceID *uuid.UUID
	)

	err := row.Scan(&tagUUID, &name, &tagType, &deviceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, tag.ErrTagNotFound
		}
		return nil, fmt.Errorf("error scanning tag: %w", err)
	}

	return r.reconstruct(tagUUID, name, tagType, deviceID)
}

func (r tagRepositoryPostgresImpl) FindAll(ctx context.Context) ([]tag.Tag, error) {
	return r.readTags(ctx,
		`SELECT id, name, type, device_id FROM engineering.tags WHERE project_id = $1 ORDER BY name`,
		r.project.UUID(),
	)
}

func (r tagRepositoryPostgresImpl) FindAllInLiveProject(ctx context.Context, orgID id.ID[organization.Organization]) ([]tag.Tag, error) {
	return r.readTags(ctx,
		`SELECT t.id, t.name, t.type, t.device_id
		 FROM engineering.tags t JOIN projects p ON p.id = t.project_id
		 WHERE t.project_id = $1 AND p.deleted_at IS NULL AND p.organization_id = $2
		 ORDER BY t.name`,
		r.project.UUID(), orgID.UUID(),
	)
}

// readTags reads the tags query selects, its columns those of FindAll.
func (r tagRepositoryPostgresImpl) readTags(ctx context.Context, query string, args ...any) ([]tag.Tag, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying tags: %w", err)
	}
	defer rows.Close()

	var tags []tag.Tag
	for rows.Next() {
		var (
			tagUUID   uuid.UUID
			tagName   string
			tagType   string
			tagDevice *uuid.UUID
		)
		if err := rows.Scan(&tagUUID, &tagName, &tagType, &tagDevice); err != nil {
			return nil, fmt.Errorf("error scanning tag: %w", err)
		}
		t, err := r.reconstruct(tagUUID, tagName, tagType, tagDevice)
		if err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over tags: %w", err)
	}
	return tags, nil
}

func (r tagRepositoryPostgresImpl) reconstruct(aTagUUID uuid.UUID, aName, aTagType string, aDeviceID *uuid.UUID) (tag.Tag, error) {
	newID := id.NewID(id.IDWithUUID[tag.Tag](aTagUUID))

	newName, err := tag.NewTagName(aName)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag name: %w", err)
	}

	newType, err := tag.NewTagType(aTagType)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag type: %w", err)
	}

	return tag.ReconstituteTag(newID, r.project, newName, newType, supplierOf(aDeviceID))
}

// supplierOf turns the stored device_id of a Tag (NULL when people supply it)
// into its Supplier.
func supplierOf(anUUID *uuid.UUID) tag.Supplier {
	if anUUID == nil {
		return tag.NoSupplier()
	}
	return tag.SuppliedByDevice(id.NewID(id.IDWithUUID[device.Device](*anUUID)))
}

// supplierValue is the device_id column of a Tag: NULL when it has no
// supplier.
func supplierValue(aSupplier tag.Supplier) any {
	if !aSupplier.HasDevice() {
		return nil
	}
	return aSupplier.DeviceID().UUID()
}
