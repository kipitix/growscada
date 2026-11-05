package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// sceneRepositoryPostgresImpl reads the scenes of one Project: every query
// names project_id. The scenes are entities of the Project's Draft (ADR 0016):
// their writes go through draftRepositoryPostgres.
type sceneRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
}

var _ scene.SceneRepository = (*sceneRepositoryPostgresImpl)(nil)

// NewSceneRepositoryPostgres returns the repository of the scenes of the
// Project aProjectID (ProjectScopes.ForProject is how the server opens one).
func NewSceneRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], _ ...Option) scene.SceneRepository {
	return &sceneRepositoryPostgresImpl{db: aDb, project: aProjectID}
}

func (r sceneRepositoryPostgresImpl) NextID() id.ID[scene.Scene] {
	return id.NewID[scene.Scene]()
}

func (r sceneRepositoryPostgresImpl) NextWidgetID() id.ID[scene.Widget] {
	return id.NewID[scene.Widget]()
}

func (r sceneRepositoryPostgresImpl) FindByID(ctx context.Context, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	found, err := r.readScenes(ctx, `AND id = $2`, sceneID.UUID())
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, id.NotFound(scene.ErrSceneNotFound, sceneID)
	}
	return found[0], nil
}

func (r sceneRepositoryPostgresImpl) FindAll(ctx context.Context) ([]scene.Scene, error) {
	return r.readScenes(ctx, ``)
}

// inLiveProject is the condition of readScenes that the scenes' Project is
// live and of the Organization numbered $n.
func inLiveProject(n int) string {
	return fmt.Sprintf(`AND EXISTS (SELECT 1 FROM projects p WHERE p.id = scenes.project_id
	   AND p.deleted_at IS NULL AND p.organization_id = $%d)`, n)
}

func (r sceneRepositoryPostgresImpl) FindByIDInLiveProject(ctx context.Context, sceneID id.ID[scene.Scene], orgID id.ID[organization.Organization]) (scene.Scene, error) {
	found, err := r.readScenes(ctx, `AND id = $2 `+inLiveProject(3), sceneID.UUID(), orgID.UUID())
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, id.NotFound(scene.ErrSceneNotFound, sceneID)
	}
	return found[0], nil
}

func (r sceneRepositoryPostgresImpl) FindAllInLiveProject(ctx context.Context, orgID id.ID[organization.Organization]) ([]scene.Scene, error) {
	return r.readScenes(ctx, inLiveProject(2), orgID.UUID())
}

// FindByTagID matches a widget by containment: its port_bindings array holds
// an object whose tag_id is the Tag's, whatever its port_name.
func (r sceneRepositoryPostgresImpl) FindByTagID(ctx context.Context, tagID id.ID[tag.Tag]) ([]scene.Scene, error) {
	return r.readScenes(ctx,
		`AND EXISTS (SELECT 1 FROM engineering.widgets w WHERE w.scene_id = scenes.id
		   AND w.port_bindings @> jsonb_build_array(jsonb_build_object('tag_id', $2::text)))`,
		tagID.UUID().String(),
	)
}

// selectSceneColumns lists the scenes columns read by every SELECT in this
// file; the order matches rawSceneRow.dest.
const selectSceneColumns = `id, name, width, height, background_html`

// rawSceneRow holds the raw column values read from a scenes row before domain validation.
type rawSceneRow struct {
	id             uuid.UUID
	name           string
	width, height  int
	backgroundHTML string
}

func (row *rawSceneRow) dest() []any {
	return []any{&row.id, &row.name, &row.width, &row.height, &row.backgroundHTML}
}

// readScenes reads the scenes of the Project matching and, a further
// condition whose arguments are numbered from $2, in creation order, with
// their widgets. Both queries run in one read-only REPEATABLE READ
// transaction, so they see the database at the same moment: a scene and its
// widgets always belong to one write.
func (r sceneRepositoryPostgresImpl) readScenes(ctx context.Context, and string, args ...any) ([]scene.Scene, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT `+selectSceneColumns+` FROM engineering.scenes WHERE project_id = $1 `+and+` ORDER BY pk_id`,
		append([]any{r.project.UUID()}, args...)...,
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
		reconstructed, err := reconstructScene(r.project, row, widgetsByScene[row.id])
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
		"SELECT "+selectWidgetColumns+" FROM engineering.widgets WHERE scene_id = ANY($1::uuid[]) ORDER BY pk_id",
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

func reconstructScene(projectID id.ID[project.Project], row rawSceneRow, someWidgets []scene.Widget) (scene.Scene, error) {
	newID := id.NewID(id.IDWithUUID[scene.Scene](row.id))

	newName, err := scene.NewSceneName(row.name)
	if err != nil {
		return nil, fmt.Errorf("cannot create scene name: %w", err)
	}

	newSize, err := scene.NewSceneSize(row.width, row.height)
	if err != nil {
		return nil, fmt.Errorf("cannot create scene size: %w", err)
	}

	return scene.ReconstituteScene(newID, projectID, newName, newSize, scene.NewBackgroundHTML(row.backgroundHTML), someWidgets), nil
}

// checkPinned reports whether the WidgetType typeID is held by a Library
// Release the Project projectID pins: what a write of a Scene checks in the
// Draft's transaction (ADR 0012).
func checkPinned(ctx context.Context, q rowQuerier, projectID id.ID[project.Project], typeID id.ID[library.WidgetType]) error {
	var pinned bool
	err := q.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM engineering.library_pins lp
		   JOIN engineering.released_widget_types r
		     ON r.library_id = lp.library_id AND r.release_number = lp.release_number
		   WHERE lp.project_id = $1 AND r.id = $2)`,
		projectID.UUID(), typeID.UUID(),
	).Scan(&pinned)
	if err != nil {
		return fmt.Errorf("cannot check the pin of widget type %s: %w", typeID, err)
	}
	if !pinned {
		return id.NotFound(library.ErrWidgetTypeNotFound, typeID)
	}
	return nil
}
