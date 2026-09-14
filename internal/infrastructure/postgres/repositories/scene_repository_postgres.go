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
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
)

type sceneRepositoryPostgresImpl struct {
	db *sql.DB
}

var _ scene.SceneRepository = (*sceneRepositoryPostgresImpl)(nil)

func NewSceneRepositoryPostgres(aDb *sql.DB) scene.SceneRepository {
	return &sceneRepositoryPostgresImpl{db: aDb}
}

func (r sceneRepositoryPostgresImpl) NextID() id.ID[scene.Scene] {
	return id.NewID[scene.Scene]()
}

func (r sceneRepositoryPostgresImpl) NextWidgetID() id.ID[widget.Widget] {
	return id.NewID[widget.Widget]()
}

func (r sceneRepositoryPostgresImpl) Save(ctx context.Context, s scene.Scene) (scene.Scene, error) {
	if s.Version() == version.Initial[scene.Scene]() {
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO scenes (id, name, width, height, background_html, version)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			s.ID().UUID(), s.Name().String(),
			s.Size().Width(), s.Size().Height(),
			s.BackgroundHTML().Content(),
			version.Committed[scene.Scene]().Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot insert new scene: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on insert: %w", err)
		}
		if rowsAffected != 1 {
			return nil, fmt.Errorf("expected 1 row affected on insert, got %d", rowsAffected)
		}
		return scene.NewScene(s.ID(), s.Name(), s.Size(), s.BackgroundHTML(), nil, version.Committed[scene.Scene]()), nil
	}

	if s.Version().IsCommitted() {
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE scenes
			 SET name = $1, width = $2, height = $3, background_html = $4, version = version + 1
			 WHERE id = $5 AND version = $6`,
			s.Name().String(),
			s.Size().Width(), s.Size().Height(),
			s.BackgroundHTML().Content(),
			s.ID().UUID(), s.Version().Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot update scene: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on update: %w", err)
		}
		if rowsAffected != 1 {
			return nil, classifyUpdateConflict[scene.Scene](ctx, r.db, tableNameScenes, s.ID(), scene.ErrSceneNotFound, scene.ErrSceneConflict)
		}
		return scene.NewScene(s.ID(), s.Name(), s.Size(), s.BackgroundHTML(), s.Widgets(), s.Version().Next()), nil
	}

	return nil, fmt.Errorf("undefined behavior with version %d", s.Version().Number())
}

func (r sceneRepositoryPostgresImpl) FindByID(ctx context.Context, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	var (
		rawID          uuid.UUID
		name           string
		width, height  int
		backgroundHTML string
		ver            int
	)

	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, width, height, background_html, version FROM scenes WHERE id = $1`,
		sceneID.UUID(),
	)
	err := row.Scan(&rawID, &name, &width, &height, &backgroundHTML, &ver)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, scene.ErrSceneNotFound
		}
		return nil, fmt.Errorf("error scanning scene: %w", err)
	}

	widgets, err := r.findWidgetsBySceneIDTx(ctx, r.db, sceneID)
	if err != nil {
		return nil, err
	}

	return r.reconstruct(rawID, name, width, height, backgroundHTML, ver, widgets)
}

func (r sceneRepositoryPostgresImpl) DeleteByID(ctx context.Context, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	widgets, err := r.findWidgetsBySceneIDTx(ctx, tx, sceneID)
	if err != nil {
		return nil, err
	}

	var (
		rawID          uuid.UUID
		name           string
		width, height  int
		backgroundHTML string
		ver            int
	)
	row := tx.QueryRowContext(ctx,
		`DELETE FROM scenes WHERE id = $1 RETURNING id, name, width, height, background_html, version`,
		sceneID.UUID(),
	)
	if err := row.Scan(&rawID, &name, &width, &height, &backgroundHTML, &ver); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, scene.ErrSceneNotFound
		}
		return nil, fmt.Errorf("cannot delete scene: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot commit scene deletion: %w", err)
	}

	return r.reconstruct(rawID, name, width, height, backgroundHTML, ver, widgets)
}

func (r sceneRepositoryPostgresImpl) FindAll(ctx context.Context) ([]scene.Scene, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, width, height, background_html, version FROM scenes`,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying scenes: %w", err)
	}
	defer rows.Close()

	type rawScene struct {
		id             uuid.UUID
		name           string
		width, height  int
		backgroundHTML string
		version        int
	}
	var rawScenes []rawScene
	for rows.Next() {
		var s rawScene
		if err := rows.Scan(&s.id, &s.name, &s.width, &s.height, &s.backgroundHTML, &s.version); err != nil {
			return nil, fmt.Errorf("error scanning scene: %w", err)
		}
		rawScenes = append(rawScenes, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over scenes: %w", err)
	}

	widgetsByScene, err := r.findAllWidgetsGroupedByScene(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]scene.Scene, 0, len(rawScenes))
	for _, s := range rawScenes {
		reconstructed, err := r.reconstruct(s.id, s.name, s.width, s.height, s.backgroundHTML, s.version, widgetsByScene[s.id])
		if err != nil {
			return nil, err
		}
		result = append(result, reconstructed)
	}
	return result, nil
}

func (r sceneRepositoryPostgresImpl) reconstruct(
	aRawID uuid.UUID, aName string, aWidth, aHeight int, aBackgroundHTML string, aVersion int, someWidgets []widget.Widget,
) (scene.Scene, error) {
	newID := id.NewID(id.IDWithUUID[scene.Scene](aRawID))

	newName, err := scene.NewSceneName(aName)
	if err != nil {
		return nil, fmt.Errorf("cannot create scene name: %w", err)
	}

	newSize, err := scene.NewSceneSize(aWidth, aHeight)
	if err != nil {
		return nil, fmt.Errorf("cannot create scene size: %w", err)
	}

	newVersion, err := version.New(version.WithNumber[scene.Scene](aVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create scene version: %w", err)
	}

	return scene.NewScene(newID, newName, newSize, scene.NewBackgroundHTML(aBackgroundHTML), someWidgets, newVersion), nil
}

// ── Widget access (Widget is an entity of the Scene aggregate) ─────────────────

// dbTX abstracts over *sql.DB and *sql.Tx for queries shared between the two.
type dbTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (r sceneRepositoryPostgresImpl) findWidgetsBySceneIDTx(ctx context.Context, tx dbTX, sceneID id.ID[scene.Scene]) ([]widget.Widget, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT "+selectWidgetColumns+" FROM widgets WHERE scene_id = $1",
		sceneID.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widgets by scene id: %w", err)
	}
	defer rows.Close()

	var result []widget.Widget
	for rows.Next() {
		w, _, err := scanWidget(rows.Scan)
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

func (r sceneRepositoryPostgresImpl) findAllWidgetsGroupedByScene(ctx context.Context) (map[uuid.UUID][]widget.Widget, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+selectWidgetColumns+" FROM widgets")
	if err != nil {
		return nil, fmt.Errorf("error querying widgets: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]widget.Widget)
	for rows.Next() {
		w, sceneID, err := scanWidget(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning widget: %w", err)
		}
		result[sceneID] = append(result[sceneID], w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over widgets: %w", err)
	}
	return result, nil
}

func (r sceneRepositoryPostgresImpl) FindWidgetsBySceneID(ctx context.Context, sceneID id.ID[scene.Scene]) ([]widget.Widget, error) {
	return r.findWidgetsBySceneIDTx(ctx, r.db, sceneID)
}

// selectWidgetInSceneColumns is selectWidgetColumns, each column qualified
// with the "w" alias so the join against scenes stays unambiguous.
const selectWidgetInSceneColumns = `w.id, w.name, w.x, w.y, w.z, w.width, w.height, w.origin_x, w.origin_y, w.rotation_degrees, w.type_id, w.scene_id, w.labels, w.port_bindings`

func (r sceneRepositoryPostgresImpl) FindWidgetsByTypeID(ctx context.Context, typeID id.ID[widget.WidgetType]) ([]scene.WidgetInScene, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectWidgetInSceneColumns+`, s.version
		 FROM widgets w JOIN scenes s ON s.id = w.scene_id
		 WHERE w.type_id = $1`,
		typeID.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widgets by type id: %w", err)
	}
	defer rows.Close()

	var result []scene.WidgetInScene
	for rows.Next() {
		var sceneVer int
		w, sceneUUID, err := scanWidget(func(dest ...any) error {
			return rows.Scan(append(dest, &sceneVer)...)
		})
		if err != nil {
			return nil, fmt.Errorf("error scanning widget: %w", err)
		}
		ver, err := version.New(version.WithNumber[scene.Scene](sceneVer))
		if err != nil {
			return nil, fmt.Errorf("cannot create scene version: %w", err)
		}
		result = append(result, scene.WidgetInScene{
			SceneID:      id.NewID(id.IDWithUUID[scene.Scene](sceneUUID)),
			SceneVersion: ver,
			Widget:       w,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over widgets: %w", err)
	}
	return result, nil
}

func (r sceneRepositoryPostgresImpl) FindWidgetByID(ctx context.Context, sceneID id.ID[scene.Scene], widgetID id.ID[widget.Widget]) (widget.Widget, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+selectWidgetColumns+" FROM widgets WHERE id = $1 AND scene_id = $2",
		widgetID.UUID(), sceneID.UUID(),
	)
	w, _, err := scanWidget(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetNotFound
		}
		return nil, fmt.Errorf("error scanning widget: %w", err)
	}
	return w, nil
}

// bumpSceneVersion checks expectedVersion against the scene's current version
// within tx and atomically bumps it, returning the new version. It returns
// scene.ErrSceneConflict/ErrSceneNotFound classified against the scenes table.
func (r sceneRepositoryPostgresImpl) bumpSceneVersion(ctx context.Context, tx *sql.Tx, sceneID id.ID[scene.Scene], expectedVersion version.Version[scene.Scene]) (version.Version[scene.Scene], error) {
	var newVer int
	err := tx.QueryRowContext(ctx,
		`UPDATE scenes SET version = version + 1 WHERE id = $1 AND version = $2 RETURNING version`,
		sceneID.UUID(), expectedVersion.Number(),
	).Scan(&newVer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return version.Version[scene.Scene]{}, classifyUpdateConflict[scene.Scene](ctx, r.db, tableNameScenes, sceneID, scene.ErrSceneNotFound, scene.ErrSceneConflict)
		}
		return version.Version[scene.Scene]{}, fmt.Errorf("cannot bump scene version: %w", err)
	}
	return version.New(version.WithNumber[scene.Scene](newVer))
}

func (r sceneRepositoryPostgresImpl) AddWidget(ctx context.Context, sceneID id.ID[scene.Scene], expectedVersion version.Version[scene.Scene], w widget.Widget) (widget.Widget, version.Version[scene.Scene], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	newVer, err := r.bumpSceneVersion(ctx, tx, sceneID, expectedVersion)
	if err != nil {
		return nil, version.Version[scene.Scene]{}, err
	}

	portBindingsJSON, err := marshalPortBindings(w.PortBindings())
	if err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot serialize port bindings: %w", err)
	}

	row := tx.QueryRowContext(ctx,
		`INSERT INTO widgets
		    (id, name, x, y, z, width, height, origin_x, origin_y, rotation_degrees,
		     type_id, scene_id, labels, port_bindings)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING `+selectWidgetColumns,
		w.ID().UUID(), w.Name().String(),
		w.Position().X(), w.Position().Y(), w.Position().Z(),
		w.Size().Width(), w.Size().Height(),
		w.Origin().X(), w.Origin().Y(),
		w.Rotation().Degrees(),
		w.TypeID().UUID(), sceneID.UUID(),
		pq.Array(w.Labels()), portBindingsJSON,
	)
	saved, _, err := scanWidget(row.Scan)
	if err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot insert new widget: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot commit widget insert: %w", err)
	}

	return saved, newVer, nil
}

func (r sceneRepositoryPostgresImpl) UpdateWidget(ctx context.Context, sceneID id.ID[scene.Scene], expectedVersion version.Version[scene.Scene], w widget.Widget) (widget.Widget, version.Version[scene.Scene], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	newVer, err := r.bumpSceneVersion(ctx, tx, sceneID, expectedVersion)
	if err != nil {
		return nil, version.Version[scene.Scene]{}, err
	}

	portBindingsJSON, err := marshalPortBindings(w.PortBindings())
	if err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot serialize port bindings: %w", err)
	}

	row := tx.QueryRowContext(ctx,
		`UPDATE widgets
		 SET name = $1, x = $2, y = $3, z = $4, width = $5, height = $6,
		     origin_x = $7, origin_y = $8, rotation_degrees = $9,
		     type_id = $10, labels = $11, port_bindings = $12
		 WHERE id = $13 AND scene_id = $14
		 RETURNING `+selectWidgetColumns,
		w.Name().String(),
		w.Position().X(), w.Position().Y(), w.Position().Z(),
		w.Size().Width(), w.Size().Height(),
		w.Origin().X(), w.Origin().Y(),
		w.Rotation().Degrees(),
		w.TypeID().UUID(),
		pq.Array(w.Labels()), portBindingsJSON,
		w.ID().UUID(), sceneID.UUID(),
	)
	saved, _, err := scanWidget(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, version.Version[scene.Scene]{}, widget.ErrWidgetNotFound
		}
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot update widget: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot commit widget update: %w", err)
	}

	return saved, newVer, nil
}

func (r sceneRepositoryPostgresImpl) DeleteWidget(ctx context.Context, sceneID id.ID[scene.Scene], widgetID id.ID[widget.Widget]) (widget.Widget, version.Version[scene.Scene], error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var newVer int
	err = tx.QueryRowContext(ctx,
		`UPDATE scenes SET version = version + 1 WHERE id = $1 RETURNING version`,
		sceneID.UUID(),
	).Scan(&newVer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, version.Version[scene.Scene]{}, scene.ErrSceneNotFound
		}
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot bump scene version: %w", err)
	}
	newVersion, err := version.New(version.WithNumber[scene.Scene](newVer))
	if err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot create scene version: %w", err)
	}

	row := tx.QueryRowContext(ctx,
		"DELETE FROM widgets WHERE id = $1 AND scene_id = $2 RETURNING "+selectWidgetColumns,
		widgetID.UUID(), sceneID.UUID(),
	)
	deleted, _, err := scanWidget(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, version.Version[scene.Scene]{}, widget.ErrWidgetNotFound
		}
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot delete widget: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, version.Version[scene.Scene]{}, fmt.Errorf("cannot commit widget deletion: %w", err)
	}

	return deleted, newVersion, nil
}
