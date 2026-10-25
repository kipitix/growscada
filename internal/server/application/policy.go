package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// Policy brings state into line with the current state of what an event
// names, outside the transaction that recorded the event (ADR 0008). The
// outbox dispatcher enqueues a job for every delivered event the policy keys;
// a policy runner runs it later, again and again until it succeeds. Jobs with
// the same key merge, so Run must be idempotent and read what it brings into
// line when it runs, never take it from an event.
type Policy interface {
	// Name identifies the policy's jobs; it must not change between runs of
	// the server while jobs of the policy may wait.
	Name() string
	// JobKey reports whether the policy needs to run for e, and the key of
	// the job it needs: what it brings into line, e.g. a WidgetType's ID.
	JobKey(e event.Event) (string, bool)
	// Run brings into line what key names.
	Run(ctx context.Context, key string) error
}

// Job keys of a Project's content start with "project:<id>".
const projectKeyPrefix = "project:"

// cutProjectKey returns the Project a job key starts with and the rest of the
// key.
func cutProjectKey(key string) (id.ID[project.Project], string, error) {
	rest, ok := strings.CutPrefix(key, projectKeyPrefix)
	if !ok {
		return id.ID[project.Project]{}, "", fmt.Errorf("invalid job key %q", key)
	}
	rawProjectID, rest, _ := strings.Cut(rest, "/")
	projectUUID, err := uuid.Parse(rawProjectID)
	if err != nil {
		return id.ID[project.Project]{}, "", fmt.Errorf("invalid project ID in job key %q: %w", key, err)
	}
	if rest != "" {
		rest = "/" + rest
	}
	return id.NewID(id.IDWithUUID[project.Project](projectUUID)), rest, nil
}
