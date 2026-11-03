package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// ProjectFileRepositoryPostgres is the Engineering node's copy of the last
// ProjectFile it sent to the Runtime node (task 35): engineering.project_files,
// one row per Project. The copy is secondary — the truth is the Revision on
// the Runtime node (Q6) — and it is what tells an unchanged Draft, and what
// the Project mode shows as "Revision N · deployed at T". It goes with its
// Project (ON DELETE CASCADE).
type ProjectFileRepositoryPostgres struct {
	db *sql.DB
}

// NewProjectFileRepositoryPostgres returns the repository of the copies of the
// last ProjectFile.
func NewProjectFileRepositoryPostgres(aDb *sql.DB) *ProjectFileRepositoryPostgres {
	return &ProjectFileRepositoryPostgres{db: aDb}
}

// Save replaces the copy of the Project's last ProjectFile: a Project has one,
// the one of the Revision it runs. Two Deploys may finish in any order, so a
// write of an older Revision than the stored one changes nothing: the copy
// follows the highest numbered Revision, and an older one is no news.
func (r ProjectFileRepositoryPostgres) Save(ctx context.Context, projectID uuid.UUID, revision appdto.Revision) error {
	if revision.Content.ProjectID != projectID {
		return fmt.Errorf("cannot save the project file of project %s for project %s", revision.Content.ProjectID, projectID)
	}
	config, err := json.Marshal(revision.Content)
	if err != nil {
		return fmt.Errorf("cannot encode project file of project %s: %w", projectID, err)
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO engineering.project_files (project_id, revision_n, deployed_at, config)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (project_id) DO UPDATE
		 SET revision_n = EXCLUDED.revision_n, deployed_at = EXCLUDED.deployed_at, config = EXCLUDED.config
		 WHERE EXCLUDED.revision_n > engineering.project_files.revision_n`,
		projectID, revision.Number, revision.DeployedAt, config,
	); err != nil {
		return fmt.Errorf("cannot save the project file of project %s: %w", projectID, err)
	}
	return nil
}

// FindByProject returns the copy of the Project's last ProjectFile, and
// whether the Project has one at all: a Project is not deployed until a
// Deploy wrote it.
func (r ProjectFileRepositoryPostgres) FindByProject(ctx context.Context, projectID uuid.UUID) (appdto.Revision, bool, error) {
	var (
		number     int
		deployedAt time.Time
		config     []byte
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT revision_n, deployed_at, config FROM engineering.project_files WHERE project_id = $1`,
		projectID,
	).Scan(&number, &deployedAt, &config)
	if errors.Is(err, sql.ErrNoRows) {
		return appdto.Revision{}, false, nil
	}
	if err != nil {
		return appdto.Revision{}, false, fmt.Errorf("cannot read the project file of project %s: %w", projectID, err)
	}
	var file projectv0.ProjectFile
	if err := json.Unmarshal(config, &file); err != nil {
		return appdto.Revision{}, false, fmt.Errorf("cannot decode the project file of project %s: %w", projectID, err)
	}
	return appdto.Revision{Number: number, DeployedAt: deployedAt.UTC(), Content: file}, true, nil
}
