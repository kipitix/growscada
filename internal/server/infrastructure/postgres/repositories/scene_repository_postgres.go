package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

type sceneRepositoryPostgresImpl struct {
	db    *sql.DB
	store aggregateStore[scene.Scene]
}

var _ scene.SceneRepository = (*sceneRepositoryPostgresImpl)(nil)

func NewSceneRepositoryPostgres(aDb *sql.DB, opts ...Option) scene.SceneRepository {
	o := newOptions(opts)
	return &sceneRepositoryPostgresImpl{
		db: aDb,
		store: aggregateStore[scene.Scene]{
			db: aDb, table: tableNameScenes,
			notFound: scene.ErrSceneNotFound, conflict: scene.ErrSceneConflict,
			onCommit: o.onCommit,
		},
	}
}

func (r sceneRepositoryPostgresImpl) NextID() id.ID[scene.Scene] {
	return id.NewID[scene.Scene]()
}

func (r sceneRepositoryPostgresImpl) NextWidgetID() id.ID[scene.Widget] {
	return id.NewID[scene.Widget]()
}

// Save writes the scene row and its widgets in one transaction. On update the
// CAS on the scenes row comes first: it locks the row, so concurrent Saves of
// the same scene are serialized and the widgets read after it are the ones
// the stored version describes.
func (r sceneRepositoryPostgresImpl) Save(ctx context.Context, s scene.Scene) (scene.Scene, error) {
	var saved []scene.Widget
	// writeWidgets brings the widget rows from stored to the scene's and
	// reads them back, so that the result is what a later read returns, as
	// the stored columns hold them.
	writeWidgets := func(tx *sql.Tx, stored []scene.Widget) error {
		if err := saveWidgets(ctx, tx, s.ID(), stored, s.Widgets()); err != nil {
			return err
		}
		byScene, err := findWidgetsBySceneIDs(ctx, tx, []uuid.UUID{s.ID().UUID()})
		if err != nil {
			return err
		}
		saved = byScene[s.ID().UUID()]
		return nil
	}

	newVersion, err := r.store.save(ctx, s.ID(), s.Version(), s.PendingEvents(), rowWrite[scene.Scene]{
		insert: func(tx *sql.Tx, next version.Version[scene.Scene]) error {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO scenes (id, name, width, height, background_html, version)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				s.ID().UUID(), s.Name().String(),
				s.Size().Width(), s.Size().Height(),
				s.BackgroundHTML().Content(),
				next.Number(),
			); err != nil {
				return fmt.Errorf("cannot insert new scene: %w", err)
			}
			return writeWidgets(tx, nil)
		},
		update: func(tx *sql.Tx, current, next version.Version[scene.Scene]) (bool, error) {
			res, err := tx.ExecContext(ctx,
				`UPDATE scenes
				 SET name = $1, width = $2, height = $3, background_html = $4, version = $5
				 WHERE id = $6 AND version = $7`,
				s.Name().String(),
				s.Size().Width(), s.Size().Height(),
				s.BackgroundHTML().Content(),
				next.Number(),
				s.ID().UUID(), current.Number(),
			)
			if err != nil {
				return false, fmt.Errorf("cannot update scene: %w", err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return false, fmt.Errorf("cannot get rows affected on update: %w", err)
			}
			if n != 1 {
				return false, nil
			}
			byScene, err := findWidgetsBySceneIDs(ctx, tx, []uuid.UUID{s.ID().UUID()})
			if err != nil {
				return false, err
			}
			return true, writeWidgets(tx, byScene[s.ID().UUID()])
		},
	})
	if err != nil {
		return nil, err
	}
	return scene.ReconstituteScene(s.ID(), s.Name(), s.Size(), s.BackgroundHTML(), saved, newVersion), nil
}

// Delete removes the scene row if it is still at the scene's version; its
// widgets go by ON DELETE CASCADE. The CAS makes them the widgets the scene
// holds, so Scene.Delete recorded a WidgetDeletedEvent for each.
func (r sceneRepositoryPostgresImpl) Delete(ctx context.Context, s scene.Scene) error {
	return r.store.delete(ctx, s.ID(), s.Version(), s.PendingEvents())
}

// saveWidgets brings the scene's widget rows from stored to wanted: inserts
// the new widgets, updates the changed ones and deletes those that are gone.
func saveWidgets(ctx context.Context, tx *sql.Tx, sceneID id.ID[scene.Scene], stored, wanted []scene.Widget) error {
	storedByID := make(map[id.ID[scene.Widget]]scene.Widget, len(stored))
	for _, w := range stored {
		storedByID[w.ID()] = w
	}
	wantedIDs := make(map[id.ID[scene.Widget]]struct{}, len(wanted))
	for _, w := range wanted {
		wantedIDs[w.ID()] = struct{}{}
	}

	for _, w := range stored {
		if _, ok := wantedIDs[w.ID()]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM widgets WHERE id = $1 AND scene_id = $2`,
			w.ID().UUID(), sceneID.UUID(),
		); err != nil {
			return fmt.Errorf("cannot delete widget %s: %w", w.ID(), err)
		}
	}

	for _, w := range wanted {
		old, exists := storedByID[w.ID()]
		if exists && sameWidget(old, w) {
			continue
		}
		portBindingsJSON, err := marshalPortBindings(w.PortBindings())
		if err != nil {
			return fmt.Errorf("cannot serialize port bindings: %w", err)
		}
		if exists {
			_, err = tx.ExecContext(ctx,
				`UPDATE widgets
				 SET name = $1, x = $2, y = $3, z = $4, width = $5, height = $6,
				     origin_x = $7, origin_y = $8, rotation_degrees = $9,
				     type_id = $10, labels = $11, port_bindings = $12
				 WHERE id = $13 AND scene_id = $14`,
				w.Name().String(),
				w.Position().X(), w.Position().Y(), w.Position().Z(),
				w.Size().Width(), w.Size().Height(),
				w.Origin().X(), w.Origin().Y(),
				w.Rotation().Degrees(),
				w.TypeID().UUID(),
				pq.Array(w.Labels()), portBindingsJSON,
				w.ID().UUID(), sceneID.UUID(),
			)
		} else {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO widgets
				    (id, name, x, y, z, width, height, origin_x, origin_y, rotation_degrees,
				     type_id, scene_id, labels, port_bindings)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
				w.ID().UUID(), w.Name().String(),
				w.Position().X(), w.Position().Y(), w.Position().Z(),
				w.Size().Width(), w.Size().Height(),
				w.Origin().X(), w.Origin().Y(),
				w.Rotation().Degrees(),
				w.TypeID().UUID(), sceneID.UUID(),
				pq.Array(w.Labels()), portBindingsJSON,
			)
		}
		if err != nil {
			if isForeignKeyViolation(err, constraintWidgetsTypeID) {
				return library.ErrWidgetTypeNotFound
			}
			return fmt.Errorf("cannot save widget %s: %w", w.ID(), err)
		}
	}
	return nil
}

// sameWidget reports whether a and b would be stored as the same row.
func sameWidget(a, b scene.Widget) bool {
	return a.Name() == b.Name() &&
		a.Position() == b.Position() &&
		a.Size() == b.Size() &&
		a.Origin() == b.Origin() &&
		a.Rotation() == b.Rotation() &&
		a.TypeID() == b.TypeID() &&
		slices.Equal(a.Labels(), b.Labels()) &&
		slices.Equal(a.PortBindings(), b.PortBindings())
}

func (r sceneRepositoryPostgresImpl) FindByID(ctx context.Context, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	found, err := r.readScenes(ctx, `WHERE id = $1`, sceneID.UUID())
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, scene.ErrSceneNotFound
	}
	return found[0], nil
}

func (r sceneRepositoryPostgresImpl) FindAll(ctx context.Context) ([]scene.Scene, error) {
	return r.readScenes(ctx, ``)
}

func (r sceneRepositoryPostgresImpl) FindByWidgetTypeID(ctx context.Context, typeID id.ID[library.WidgetType]) ([]scene.Scene, error) {
	return r.readScenes(ctx,
		`WHERE EXISTS (SELECT 1 FROM widgets w WHERE w.scene_id = scenes.id AND w.type_id = $1)`,
		typeID.UUID(),
	)
}

// selectSceneColumns lists the scenes columns read by every SELECT in this
// file; the order matches rawSceneRow.dest.
const selectSceneColumns = `id, name, width, height, background_html, version`

// rawSceneRow holds the raw column values read from a scenes row before domain validation.
type rawSceneRow struct {
	id             uuid.UUID
	name           string
	width, height  int
	backgroundHTML string
	version        int
}

func (row *rawSceneRow) dest() []any {
	return []any{&row.id, &row.name, &row.width, &row.height, &row.backgroundHTML, &row.version}
}

// readScenes reads the scenes matching where, in creation order, with their
// widgets. Both queries run in one read-only REPEATABLE READ transaction, so
// they see the database at the same moment: a scene and its widgets always
// belong to one version.
func (r sceneRepositoryPostgresImpl) readScenes(ctx context.Context, where string, args ...any) ([]scene.Scene, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT `+selectSceneColumns+` FROM scenes `+where+` ORDER BY pk_id`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying scenes: %w", err)
	}
	defer rows.Close()

	var rawScenes []rawSceneRow
	for rows.Next() {
		var row rawSceneRow
		if err := rows.Scan(row.dest()...); err != nil {
			return nil, fmt.Errorf("error scanning scene: %w", err)
		}
		rawScenes = append(rawScenes, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over scenes: %w", err)
	}
	if len(rawScenes) == 0 {
		return []scene.Scene{}, nil
	}

	sceneIDs := make([]uuid.UUID, len(rawScenes))
	for i, row := range rawScenes {
		sceneIDs[i] = row.id
	}
	widgetsByScene, err := findWidgetsBySceneIDs(ctx, tx, sceneIDs)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot commit scene read: %w", err)
	}

	result := make([]scene.Scene, 0, len(rawScenes))
	for _, row := range rawScenes {
		reconstructed, err := reconstructScene(row, widgetsByScene[row.id])
		if err != nil {
			return nil, err
		}
		result = append(result, reconstructed)
	}
	return result, nil
}

// findWidgetsBySceneIDs reads the widgets of the given scenes, grouped by
// scene, each group in creation order.
func findWidgetsBySceneIDs(ctx context.Context, tx *sql.Tx, sceneIDs []uuid.UUID) (map[uuid.UUID][]scene.Widget, error) {
	rawIDs := make([]string, len(sceneIDs))
	for i, sceneID := range sceneIDs {
		rawIDs[i] = sceneID.String()
	}

	rows, err := tx.QueryContext(ctx,
		"SELECT "+selectWidgetColumns+" FROM widgets WHERE scene_id = ANY($1::uuid[]) ORDER BY pk_id",
		pq.Array(rawIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widgets by scene id: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]scene.Widget, len(sceneIDs))
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

func reconstructScene(row rawSceneRow, someWidgets []scene.Widget) (scene.Scene, error) {
	newID := id.NewID(id.IDWithUUID[scene.Scene](row.id))

	newName, err := scene.NewSceneName(row.name)
	if err != nil {
		return nil, fmt.Errorf("cannot create scene name: %w", err)
	}

	newSize, err := scene.NewSceneSize(row.width, row.height)
	if err != nil {
		return nil, fmt.Errorf("cannot create scene size: %w", err)
	}

	newVersion, err := version.New(version.WithNumber[scene.Scene](row.version))
	if err != nil {
		return nil, fmt.Errorf("cannot create scene version: %w", err)
	}

	return scene.ReconstituteScene(newID, newName, newSize, scene.NewBackgroundHTML(row.backgroundHTML), someWidgets, newVersion), nil
}
