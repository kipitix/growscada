package growctl

import (
	"context"
	"fmt"
	"os/user"
	"time"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// The EditLock of a Draft (ADR 0015). growctl is a client like any other: a
// run that writes a Project's or a Library's Draft checks its EditLock out
// under a key of its own and a declared name, and checks it in when it is
// done. A lock another client holds is an error naming that holder;
// --take-over takes it over. A run with nothing to change takes no lock.

// clientNameFlag is the default of --client-name.
func defaultClientName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "growctl"
}

// runKey is the key this run of growctl acts under, created once.
func (s *settings) key() string {
	if s.runKey == "" {
		s.runKey = uuid.NewString()
	}
	return s.runKey
}

// draftClient is a client of one Draft's EditLock: a Project's or a Library's.
type draftClient interface {
	StateEditLock(context.Context) (apiv0.EditLockResponse, error)
	CheckOutEditLock(context.Context, string) (apiv0.EditLockResponse, error)
	CheckInEditLock(context.Context) (apiv0.EditLockResponse, error)
	TakeOverEditLock(context.Context, string, int) (apiv0.EditLockResponse, error)
}

// lockDraft checks the Draft content serves out for this run and returns the
// function that checks it in. what names the Draft in the errors: "project
// <id>" or "library <name>". A lock another client holds is an error naming
// it, unless --take-over takes it over. A Draft this run already holds (the
// same key) is a no-op.
func (s *settings) lockDraft(ctx context.Context, content draftClient, what string) (func(), error) {
	state, err := content.StateEditLock(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s edit lock: %w", what, err)
	}
	switch {
	case state.Held && state.Holder != nil && state.Holder.Key.String() == s.key():
		return func() {}, nil
	case state.Held && s.takeOver:
		if _, err := content.TakeOverEditLock(ctx, s.clientName, state.Version); err != nil {
			return nil, fmt.Errorf("%s edit lock: %w", what, err)
		}
	case state.Held:
		since := state.Holder.Since
		if t, err := time.Parse(time.RFC3339, state.Holder.Since); err == nil {
			since = t.Local().Format("2006-01-02 15:04")
		}
		return nil, fmt.Errorf("%s is being edited by %s since %s: wait for them, or use --take-over",
			what, state.Holder.Name, since)
	default:
		if _, err := content.CheckOutEditLock(ctx, s.clientName); err != nil {
			return nil, fmt.Errorf("%s edit lock: %w", what, err)
		}
	}
	return func() {
		// A failed Check in leaves the lock to its TTL (ADR 0015); the run
		// itself has done what it was asked.
		_, _ = content.CheckInEditLock(ctx)
	}, nil
}

// lockProject checks the Draft of the project projectID out for this run.
func (s *settings) lockProject(ctx context.Context, client *Client, projectID uuid.UUID) (func(), error) {
	return s.lockDraft(ctx, client.Project(projectID), "project "+projectID.String())
}

// lockLibrary checks the Draft of the library libraryID out for this run.
func (s *settings) lockLibrary(ctx context.Context, content *apiclient.LibraryClient, name string) (func(), error) {
	return s.lockDraft(ctx, content, "library "+name)
}

// lockProjects checks the Drafts of the projects out, and returns the one
// function that checks them in. A failure checks the ones taken so far back
// in before it returns.
func (s *settings) lockProjects(ctx context.Context, client *Client, projectIDs []uuid.UUID) (func(), error) {
	var unlocks []func()
	unlock := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	for _, projectID := range projectIDs {
		release, err := s.lockProject(ctx, client, projectID)
		if err != nil {
			unlock()
			return nil, err
		}
		unlocks = append(unlocks, release)
	}
	return unlock, nil
}

// lockFoundLibraries checks the Drafts of the named libraries out, looking
// each up by name, and returns the one function that checks them in. A failure
// checks the ones taken so far back in before it returns.
func (s *settings) lockFoundLibraries(ctx context.Context, client *Client, names []string) (func(), error) {
	var unlocks []func()
	unlock := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	for _, name := range names {
		content, _, err := findLibrary(ctx, client, name)
		if err != nil {
			unlock()
			return nil, err
		}
		release, err := s.lockLibrary(ctx, content, name)
		if err != nil {
			unlock()
			return nil, err
		}
		unlocks = append(unlocks, release)
	}
	return unlock, nil
}
