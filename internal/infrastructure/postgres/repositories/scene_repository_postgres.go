package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/version"
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
		return scene.NewScene(s.ID(), s.Name(), s.Size(), s.BackgroundHTML(), version.Committed[scene.Scene]()), nil
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
			return nil, r.classifyUpdateConflict(ctx, s.ID())
		}
		return scene.NewScene(s.ID(), s.Name(), s.Size(), s.BackgroundHTML(), s.Version().Next()), nil
	}

	return nil, fmt.Errorf("undefined behavior with version %d", s.Version().Number())
}

func (r sceneRepositoryPostgresImpl) classifyUpdateConflict(ctx context.Context, sceneID id.ID[scene.Scene]) error {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM scenes WHERE id = $1)`, sceneID.UUID(),
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("cannot check scene existence: %w", err)
	}
	if !exists {
		return scene.ErrSceneNotFound
	}
	return scene.ErrSceneConflict
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

	return r.reconstruct(rawID, name, width, height, backgroundHTML, ver)
}

func (r sceneRepositoryPostgresImpl) DeleteByID(ctx context.Context, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	var (
		rawID          uuid.UUID
		name           string
		width, height  int
		backgroundHTML string
		ver            int
	)

	row := r.db.QueryRowContext(ctx,
		`DELETE FROM scenes WHERE id = $1 RETURNING id, name, width, height, background_html, version`,
		sceneID.UUID(),
	)
	err := row.Scan(&rawID, &name, &width, &height, &backgroundHTML, &ver)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, scene.ErrSceneNotFound
		}
		return nil, fmt.Errorf("cannot delete scene: %w", err)
	}

	return r.reconstruct(rawID, name, width, height, backgroundHTML, ver)
}

func (r sceneRepositoryPostgresImpl) FindAll(ctx context.Context) ([]scene.Scene, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, width, height, background_html, version FROM scenes`,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying scenes: %w", err)
	}
	defer rows.Close()

	var result []scene.Scene
	for rows.Next() {
		var (
			rawID          uuid.UUID
			name           string
			width, height  int
			backgroundHTML string
			ver            int
		)
		if err := rows.Scan(&rawID, &name, &width, &height, &backgroundHTML, &ver); err != nil {
			return nil, fmt.Errorf("error scanning scene: %w", err)
		}
		s, err := r.reconstruct(rawID, name, width, height, backgroundHTML, ver)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over scenes: %w", err)
	}
	return result, nil
}

func (r sceneRepositoryPostgresImpl) reconstruct(
	aRawID uuid.UUID, aName string, aWidth, aHeight int, aBackgroundHTML string, aVersion int,
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

	return scene.NewScene(newID, newName, newSize, scene.NewBackgroundHTML(aBackgroundHTML), newVersion), nil
}
