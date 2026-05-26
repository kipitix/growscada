package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
)

type widgetRepositoryPostgresImpl struct {
	db *sql.DB
}

var _ widget.WidgetRepository = (*widgetRepositoryPostgresImpl)(nil)

func NewWidgetRepositoryPostgres(aDb *sql.DB) widget.WidgetRepository {
	return &widgetRepositoryPostgresImpl{db: aDb}
}

func (r widgetRepositoryPostgresImpl) NextID() id.ID[widget.Widget] {
	return id.NewID[widget.Widget]()
}

func (r widgetRepositoryPostgresImpl) Save(ctx context.Context, w widget.Widget) (widget.Widget, error) {
	tagIDStrings := make([]string, len(w.TagIDs()))
	for i, tid := range w.TagIDs() {
		tagIDStrings[i] = tid.UUID().String()
	}

	if w.Version() == version.Initial[widget.Widget]() {
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO widgets
			    (id, name, x, y, z, width, height, origin_x, origin_y, rotation_degrees,
			     type_id, scene_id, labels, tag_ids, version)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			w.ID().UUID(), w.Name().String(),
			w.Position().X(), w.Position().Y(), w.Position().Z(),
			w.Size().Width(), w.Size().Height(),
			w.Origin().X(), w.Origin().Y(),
			w.Rotation().Degrees(),
			w.TypeID().UUID(), nullableUUID(w.SceneID().UUID()),
			pq.Array(w.Labels()), pq.Array(tagIDStrings),
			version.Committed[widget.Widget]().Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot insert new widget: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on insert: %w", err)
		}
		if rowsAffected != 1 {
			return nil, fmt.Errorf("expected 1 row affected on insert, got %d", rowsAffected)
		}
		return widget.NewWidget(
			w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
			w.TypeID(), w.SceneID(), w.Labels(), w.TagIDs(),
			version.Committed[widget.Widget](),
		), nil
	}

	if w.Version().IsCommitted() {
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE widgets
			 SET name = $1, x = $2, y = $3, z = $4, width = $5, height = $6,
			     origin_x = $7, origin_y = $8, rotation_degrees = $9,
			     type_id = $10, scene_id = $11, labels = $12, tag_ids = $13,
			     version = version + 1
			 WHERE id = $14 AND version = $15`,
			w.Name().String(),
			w.Position().X(), w.Position().Y(), w.Position().Z(),
			w.Size().Width(), w.Size().Height(),
			w.Origin().X(), w.Origin().Y(),
			w.Rotation().Degrees(),
			w.TypeID().UUID(), nullableUUID(w.SceneID().UUID()),
			pq.Array(w.Labels()), pq.Array(tagIDStrings),
			w.ID().UUID(), w.Version().Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot update widget: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on update: %w", err)
		}
		if rowsAffected != 1 {
			return nil, r.classifyUpdateConflict(ctx, w.ID())
		}
		return widget.NewWidget(
			w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
			w.TypeID(), w.SceneID(), w.Labels(), w.TagIDs(),
			w.Version().Next(),
		), nil
	}

	return nil, fmt.Errorf("undefined behavior with version %d", w.Version().Number())
}

// nullableUUID returns nil for uuid.Nil (no scene assigned) and a pointer otherwise.
func nullableUUID(u uuid.UUID) *uuid.UUID {
	if u == uuid.Nil {
		return nil
	}
	return &u
}

func (r widgetRepositoryPostgresImpl) classifyUpdateConflict(ctx context.Context, widgetID id.ID[widget.Widget]) error {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM widgets WHERE id = $1)`, widgetID.UUID(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("cannot check widget existence: %w", err)
	}
	if !exists {
		return widget.ErrWidgetNotFound
	}
	return widget.ErrWidgetConflict
}

const selectWidgetColumns = `
	id, name, x, y, z, width, height, origin_x, origin_y, rotation_degrees,
	type_id, scene_id, labels, tag_ids::text[], version`

func (r widgetRepositoryPostgresImpl) FindByID(ctx context.Context, widgetID id.ID[widget.Widget]) (widget.Widget, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT`+selectWidgetColumns+` FROM widgets WHERE id = $1`,
		widgetID.UUID(),
	)
	w, err := r.scanWidget(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetNotFound
		}
		return nil, fmt.Errorf("error scanning widget: %w", err)
	}
	return w, nil
}

func (r widgetRepositoryPostgresImpl) DeleteByID(ctx context.Context, widgetID id.ID[widget.Widget]) (widget.Widget, error) {
	row := r.db.QueryRowContext(ctx,
		`DELETE FROM widgets WHERE id = $1 RETURNING`+selectWidgetColumns,
		widgetID.UUID(),
	)
	w, err := r.scanWidget(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetNotFound
		}
		return nil, fmt.Errorf("cannot delete widget: %w", err)
	}
	return w, nil
}

func (r widgetRepositoryPostgresImpl) FindAll(ctx context.Context) ([]widget.Widget, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT`+selectWidgetColumns+` FROM widgets`,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widgets: %w", err)
	}
	defer rows.Close()

	var result []widget.Widget
	for rows.Next() {
		w, err := r.scanWidget(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning widget: %w", err)
		}
		result = append(result, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over widgets: %w", err)
	}
	return result, nil
}

// scanWidget reads a single widget row using the provided scan function.
// The column order must match [selectWidgetColumns].
func (r widgetRepositoryPostgresImpl) scanWidget(
	scan func(...any) error,
) (widget.Widget, error) {
	var (
		rawID            uuid.UUID
		name             string
		x, y             float64
		z                int
		w, h             int
		originX, originY float64
		rotDegrees       float64
		typeID           uuid.UUID
		sceneID          *uuid.UUID
		labels           pq.StringArray
		rawTagIDs        pq.StringArray
		ver              int
	)

	if err := scan(
		&rawID, &name, &x, &y, &z, &w, &h,
		&originX, &originY, &rotDegrees,
		&typeID, &sceneID, &labels, &rawTagIDs, &ver,
	); err != nil {
		return nil, err
	}

	return r.reconstruct(rawID, name, x, y, z, w, h, originX, originY, rotDegrees, typeID, sceneID, labels, rawTagIDs, ver)
}

func (r widgetRepositoryPostgresImpl) reconstruct(
	aRawID uuid.UUID, aName string,
	aX, aY float64, aZ int,
	aWidth, aHeight int,
	anOriginX, anOriginY float64,
	aRotDegrees float64,
	aTypeID uuid.UUID, aSceneID *uuid.UUID,
	someLabels pq.StringArray, someTagIDs pq.StringArray,
	aVersion int,
) (widget.Widget, error) {
	newID := id.NewID(id.IDWithUUID[widget.Widget](aRawID))

	newName, err := widget.NewWidgetName(aName)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget name: %w", err)
	}

	pos := widget.NewPosition(aX, aY, aZ)

	size, err := widget.NewSize(aWidth, aHeight)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget size: %w", err)
	}

	origin, err := widget.NewOrigin(anOriginX, anOriginY)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget origin: %w", err)
	}

	rotation := widget.NewRotation(aRotDegrees)

	typeID := id.NewID(id.IDWithUUID[widget.WidgetType](aTypeID))

	var sceneUUID uuid.UUID
	if aSceneID != nil {
		sceneUUID = *aSceneID
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](sceneUUID))

	tagIDs := make([]id.ID[tag.Tag], len(someTagIDs))
	for i, s := range someTagIDs {
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("cannot parse tag id %q: %w", s, err)
		}
		tagIDs[i] = id.NewID(id.IDWithUUID[tag.Tag](u))
	}

	newVersion, err := version.New(version.WithNumber[widget.Widget](aVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create widget version: %w", err)
	}

	return widget.NewWidget(newID, newName, pos, size, origin, rotation, typeID, sceneID, someLabels, tagIDs, newVersion), nil
}
