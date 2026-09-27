package repositories

import (
	"errors"

	"github.com/lib/pq"
)

// constraintTagsName is the unique constraint on tags.name.
const constraintTagsName = "uq_tags_name"

// uniqueViolationCode is the PostgreSQL SQLSTATE for unique_violation.
const uniqueViolationCode = "23505"

// isUniqueViolation reports whether err is a unique_violation of the given constraint.
func isUniqueViolation(err error, constraint string) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == uniqueViolationCode && pqErr.Constraint == constraint
}
