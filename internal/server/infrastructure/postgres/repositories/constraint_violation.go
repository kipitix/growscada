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
// project that still has content: Tag definitions, Scenes and Library Pins
// in the Draft, Tag states in runtime (ADR 0011).
const (
	constraintTagsProjectID        = "fk_tags_project_id"
	constraintScenesProjectID      = "fk_scenes_project_id"
	constraintLibraryPinsProjectID = "fk_library_pins_project_id"
	constraintRuntimeTagsProjectID = "fk_runtime_tags_project_id"
)

// constraintLibrariesLiveName is the unique index on the names of the live
// libraries of an organization.
const constraintLibrariesLiveName = "uq_libraries_organization_live_name"

// constraintWidgetTypesName is the unique constraint on widget_types
// (library_id, name).
const constraintWidgetTypesName = "uq_widget_types_library_name"

// constraintReleasesNumber is the unique constraint on library_releases
// (library_id, number).
const constraintReleasesNumber = "uq_library_releases_library_number"

// Constraints of library_pins: one pin per project and library, the release
// pinned exists, and the foreign keys that refuse removing a library a
// project pins.
const (
	constraintLibraryPinsProjectLibrary = "uq_library_pins_project_library"
	constraintLibraryPinsRelease        = "fk_library_pins_release"
	constraintLibraryPinsLibraryID      = "fk_library_pins_library_id"
)

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
