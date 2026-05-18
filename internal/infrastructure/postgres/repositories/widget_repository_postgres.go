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
	newID, _ := id.NewID[widget.Widget]()
	return newID
}

func (r widgetRepositoryPostgresImpl) Save(ctx context.Context, w widget.Widget) (widget.Widget, error) {
	tagIDStrings := make([]string, len(w.TagIDs()))
	for i, tid := range w.TagIDs() {
		tagIDStrings[i] = tid.UUID().String()
	}

	if w.Version() == version.Initial[widget.Widget]() {
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO widgets (id, name, x, y, z, width, height, type_id, scene_id, labels, tag_ids, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			w.ID().UUID(), w.Name().String(),
			w.Coordinates().X(), w.Coordinates().Y(), w.Coordinates().Z(),
			w.Size().Width(), w.Size().Height(),
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
		return widget.NewWidget(w.ID(), w.Name(), w.Coordinates(), w.Size(), w.TypeID(), w.SceneID(), w.Labels(), w.TagIDs(), version.Committed[widget.Widget]())
	}

	if w.Version().IsCommitted() {
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE widgets
			 SET name = $1, x = $2, y = $3, z = $4, width = $5, height = $6,
			     type_id = $7, scene_id = $8, labels = $9, tag_ids = $10, version = version + 1
			 WHERE id = $11 AND version = $12`,
			w.Name().String(),
			w.Coordinates().X(), w.Coordinates().Y(), w.Coordinates().Z(),
			w.Size().Width(), w.Size().Height(),
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
		return widget.NewWidget(w.ID(), w.Name(), w.Coordinates(), w.Size(), w.TypeID(), w.SceneID(), w.Labels(), w.TagIDs(), w.Version().Next())
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

func (r widgetRepositoryPostgresImpl) FindByID(ctx context.Context, widgetID id.ID[widget.Widget]) (widget.Widget, error) {
	var (
		rawID     uuid.UUID
		name      string
		x, y, z   float64
		w, h      int
		typeID    uuid.UUID
		sceneID   *uuid.UUID
		labels    pq.StringArray
		rawTagIDs pq.StringArray
		ver       int
	)

	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, x, y, z, width, height, type_id, scene_id, labels, tag_ids::text[], version FROM widgets WHERE id = $1`,
		widgetID.UUID(),
	)
	err := row.Scan(&rawID, &name, &x, &y, &z, &w, &h, &typeID, &sceneID, &labels, &rawTagIDs, &ver)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetNotFound
		}
		return nil, fmt.Errorf("error scanning widget: %w", err)
	}

	return r.reconstruct(rawID, name, x, y, z, w, h, typeID, sceneID, labels, rawTagIDs, ver)
}

func (r widgetRepositoryPostgresImpl) DeleteByID(ctx context.Context, widgetID id.ID[widget.Widget]) (widget.Widget, error) {
	var (
		rawID     uuid.UUID
		name      string
		x, y, z   float64
		w, h      int
		typeID    uuid.UUID
		sceneID   *uuid.UUID
		labels    pq.StringArray
		rawTagIDs pq.StringArray
		ver       int
	)

	row := r.db.QueryRowContext(ctx,
		`DELETE FROM widgets WHERE id = $1 RETURNING id, name, x, y, z, width, height, type_id, scene_id, labels, tag_ids::text[], version`,
		widgetID.UUID(),
	)
	err := row.Scan(&rawID, &name, &x, &y, &z, &w, &h, &typeID, &sceneID, &labels, &rawTagIDs, &ver)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetNotFound
		}
		return nil, fmt.Errorf("cannot delete widget: %w", err)
	}

	return r.reconstruct(rawID, name, x, y, z, w, h, typeID, sceneID, labels, rawTagIDs, ver)
}

func (r widgetRepositoryPostgresImpl) FindAll(ctx context.Context) ([]widget.Widget, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, x, y, z, width, height, type_id, scene_id, labels, tag_ids::text[], version FROM widgets`,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widgets: %w", err)
	}
	defer rows.Close()

	var result []widget.Widget
	for rows.Next() {
		var (
			rawID     uuid.UUID
			name      string
			x, y, z   float64
			w, h      int
			typeID    uuid.UUID
			sceneID   *uuid.UUID
			labels    pq.StringArray
			rawTagIDs pq.StringArray
			ver       int
		)
		if err := rows.Scan(&rawID, &name, &x, &y, &z, &w, &h, &typeID, &sceneID, &labels, &rawTagIDs, &ver); err != nil {
			return nil, fmt.Errorf("error scanning widget: %w", err)
		}
		widget, err := r.reconstruct(rawID, name, x, y, z, w, h, typeID, sceneID, labels, rawTagIDs, ver)
		if err != nil {
			return nil, err
		}
		result = append(result, widget)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over widgets: %w", err)
	}
	return result, nil
}

func (r widgetRepositoryPostgresImpl) reconstruct(
	aRawID uuid.UUID, aName string, aX, aY, aZ float64,
	aWidth, aHeight int,
	aTypeID uuid.UUID, aSceneID *uuid.UUID, someLabels pq.StringArray, someTagIDs pq.StringArray, aVersion int,
) (widget.Widget, error) {
	newID, _ := id.NewID(id.IDWithUUID[widget.Widget](aRawID))

	newName, err := widget.NewWidgetName(aName)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget name: %w", err)
	}

	coords := widget.NewCoordinates(aX, aY, aZ)

	size, err := widget.NewSize(aWidth, aHeight)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget size: %w", err)
	}

	typeID, err := id.NewID(id.IDWithUUID[widget.WidgetType](aTypeID))
	if err != nil {
		return nil, fmt.Errorf("cannot create widget type id: %w", err)
	}

	var sceneUUID uuid.UUID
	if aSceneID != nil {
		sceneUUID = *aSceneID
	}
	sceneID, _ := id.NewID(id.IDWithUUID[scene.Scene](sceneUUID))

	tagIDs := make([]id.ID[tag.Tag], len(someTagIDs))
	for i, s := range someTagIDs {
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("cannot parse tag id %q: %w", s, err)
		}
		tid, err := id.NewID(id.IDWithUUID[tag.Tag](u))
		if err != nil {
			return nil, fmt.Errorf("cannot create tag id: %w", err)
		}
		tagIDs[i] = tid
	}

	newVersion, err := version.New(version.WithNumber[widget.Widget](aVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create widget version: %w", err)
	}

	return widget.NewWidget(newID, newName, coords, size, typeID, sceneID, someLabels, tagIDs, newVersion)
}
