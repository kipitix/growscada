package repositories

import (
	"errors"

	"github.com/lib/pq"
)

// constraintTagsName is the unique constraint on tags (project_id, name).
const constraintTagsName = "uq_tags_project_name"

// constraintProjectsLiveName is the unique index on the names of the live
// projects of an organization.
const constraintProjectsLiveName = "uq_projects_organization_live_name"

// Foreign keys of a project's content to projects; they refuse removing a
// project that still has content.
const (
	constraintTagsProjectID   = "fk_tags_project_id"
	constraintScenesProjectID = "fk_scenes_project_id"
)

// constraintWidgetsTypeID is the foreign key widgets.type_id → widget_types.
const constraintWidgetsTypeID = "fk_widgets_type_id"

// PostgreSQL SQLSTATEs for unique_violation, foreign_key_violation and
// restrict_violation. Since PostgreSQL 18 deleting a row an ON DELETE RESTRICT
// foreign key still references fails with restrict_violation, before it with
// foreign_key_violation.
const (
	uniqueViolationCode     = "23505"
	foreignKeyViolationCode = "23503"
	restrictViolationCode   = "23001"
)

// isUniqueViolation reports whether err is a unique_violation of the given constraint.
func isUniqueViolation(err error, constraint string) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == uniqueViolationCode && pqErr.Constraint == constraint
}

// isForeignKeyViolation reports whether err is a violation of the given
// foreign key: a missing referenced row on insert/update, or a still
// referenced row on delete.
func isForeignKeyViolation(err error, constraint string) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Constraint == constraint &&
		(pqErr.Code == foreignKeyViolationCode || pqErr.Code == restrictViolationCode)
}
