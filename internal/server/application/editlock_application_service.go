package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

const (
	// EditLockTTL is how long a held EditLock outlives the last sign of its
	// holder: an SSE stream with its key counts as alive continuously, and
	// so does a request with its key, which raises last_seen. A client with
	// no stream (growctl, Bruno) loses its lock this long after its last
	// write (ADR 0015).
	EditLockTTL = 5 * time.Minute

	// editLockReapInterval is how often the reaper looks at the held locks.
	editLockReapInterval = 30 * time.Second
)

// EditLockService is the Engineering node's EditLock (ADR 0015): the one
// editor of a Draft — a Project's or a Library's — identified by the key of
// the client that holds it. Presence — the keys with an open SSE stream — is
// kept in memory of this node, like the event bus.
type EditLockService interface {
	// State is the lock of the live Draft's scope, free or held. A Draft
	// never checked out is free at version 0.
	State(ctx context.Context, scope editlock.Scope) (appdto.EditLock, error)
	// CheckOut takes the lock for the client of the context under the
	// declared name, and reports whether it took it now (false: this client
	// held it already). A held lock is editlock.ErrEditLockHeld, a Draft
	// being discarded is project.ErrProjectDiscarding or
	// library.ErrLibraryDiscarding.
	CheckOut(ctx context.Context, scope editlock.Scope, name string) (appdto.EditLock, bool, error)
	// CheckIn gives the lock back to the Draft as it is. A free lock is no
	// error; another client's lock is editlock.ErrEditLockNotHeld.
	CheckIn(ctx context.Context, scope editlock.Scope) (appdto.EditLock, error)
	// TakeOver takes a held lock over, stating the version the client
	// observed: a stale one is editlock.ErrEditLockVersionStale, a free lock
	// editlock.ErrEditLockFree.
	TakeOver(ctx context.Context, scope editlock.Scope, name string, expectedVersion int) (appdto.EditLock, error)

	// CheckHeld refuses a write of the Draft that the client of the context
	// may not make: the lock is free or another client's
	// (editlock.ErrEditLockNotHeld), or the context names no client
	// (editlock.ErrNoClient). The server's own writes pass. Deploy, whose
	// build has no single transaction, checks it before it builds.
	CheckHeld(ctx context.Context, scope editlock.Scope) error

	// Attach counts an SSE stream of the client key as open: while one is,
	// the lock of that client is alive.
	Attach(key editlock.Key)
	// Detach counts a stream of the client key as closed.
	Detach(key editlock.Key)

	// ReapOnce refreshes the last_seen of every present holder and frees the
	// locks of absent holders not seen for EditLockTTL.
	ReapOnce(ctx context.Context) error
}

type editLockServiceImpl struct {
	content   ProjectContent
	libraries LibraryContent
	locks     editlock.Repository
	now       func() time.Time

	mu       sync.Mutex
	presence map[editlock.Key]int
}

var _ EditLockService = (*editLockServiceImpl)(nil)

// EditLockOption configures the EditLockService.
type EditLockOption func(*editLockServiceImpl)

// WithEditLockClock makes the service read the time from now: for tests that
// must let a lock go stale without waiting out the TTL.
func WithEditLockClock(now func() time.Time) EditLockOption {
	return func(s *editLockServiceImpl) { s.now = now }
}

// NewEditLockService returns the EditLockService over the content of the live
// Projects aContent, the Libraries aLibraries and the stored locks.
func NewEditLockService(aContent ProjectContent, aLibraries LibraryContent, locks editlock.Repository, opts ...EditLockOption) EditLockService {
	s := &editLockServiceImpl{
		content:   aContent,
		libraries: aLibraries,
		locks:     locks,
		now:       time.Now,
		presence:  make(map[editlock.Key]int),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *editLockServiceImpl) State(ctx context.Context, scope editlock.Scope) (appdto.EditLock, error) {
	if err := s.open(ctx, scope, false); err != nil {
		return appdto.EditLock{}, err
	}
	return s.stateOf(ctx, scope)
}

// open checks that the Draft the scope names exists, is live and is not being
// discarded when notDiscarding is set.
func (s *editLockServiceImpl) open(ctx context.Context, scope editlock.Scope, notDiscarding bool) error {
	switch {
	case scope.IsProject():
		_, _, err := s.content.open(ctx, scope.ProjectID().UUID())
		return err
	case scope.IsLibrary():
		if notDiscarding {
			_, err := s.libraries.openNotDiscarding(ctx, scope.LibraryID().UUID())
			return err
		}
		_, err := s.libraries.open(ctx, scope.LibraryID().UUID())
		return err
	default:
		return fmt.Errorf("edit lock: the scope names no Draft")
	}
}

func (s *editLockServiceImpl) stateOf(ctx context.Context, scope editlock.Scope) (appdto.EditLock, error) {
	lock, found, err := s.locks.Find(ctx, scope)
	if err != nil {
		return appdto.EditLock{}, err
	}
	if !found {
		return appdto.NewEditLock(editlock.Snapshot{}), nil
	}
	return appdto.NewEditLock(lock.Snapshot()), nil
}

func (s *editLockServiceImpl) CheckOut(ctx context.Context, scope editlock.Scope, name string) (appdto.EditLock, bool, error) {
	if err := s.open(ctx, scope, true); err != nil {
		return appdto.EditLock{}, false, err
	}
	holder, err := s.holderOf(ctx, name)
	if err != nil {
		return appdto.EditLock{}, false, err
	}
	var state appdto.EditLock
	took := false
	err = retryOnRaces(func() error {
		lock, found, err := s.locks.Find(ctx, scope)
		if err != nil {
			return err
		}
		if !found {
			lock = editlock.CreateEditLock(scope)
		}
		acquired, err := lock.Acquire(holder, s.now())
		if err != nil {
			return err
		}
		if found {
			if acquired == lock {
				// The same client's Check out again: the lock as it is, no
				// new version and no event.
				state = appdto.NewEditLock(lock.Snapshot())
				return nil
			}
			saved, err := s.locks.Save(ctx, acquired)
			if err != nil {
				return err
			}
			took = true
			state = appdto.NewEditLock(saved.Snapshot())
			return nil
		}
		created, err := s.locks.Create(ctx, acquired)
		if err != nil {
			return err
		}
		took = true
		state = appdto.NewEditLock(created.Snapshot())
		return nil
	}, editlock.ErrEditLockVersionStale, editlock.ErrEditLockExists)
	if err != nil {
		return appdto.EditLock{}, false, err
	}
	return state, took, nil
}

func (s *editLockServiceImpl) CheckIn(ctx context.Context, scope editlock.Scope) (appdto.EditLock, error) {
	if err := s.open(ctx, scope, false); err != nil {
		return appdto.EditLock{}, err
	}
	key, err := s.clientKey(ctx)
	if err != nil {
		return appdto.EditLock{}, err
	}
	var state appdto.EditLock
	err = retryOnRace(editlock.ErrEditLockVersionStale, func() error {
		lock, found, err := s.locks.Find(ctx, scope)
		if err != nil {
			return err
		}
		if !found {
			state = appdto.NewEditLock(editlock.Snapshot{})
			return nil
		}
		released, err := lock.Release(key, s.now())
		if err != nil {
			return err
		}
		if released == lock {
			state = appdto.NewEditLock(lock.Snapshot())
			return nil
		}
		saved, err := s.locks.Save(ctx, released)
		if err != nil {
			return err
		}
		state = appdto.NewEditLock(saved.Snapshot())
		return nil
	})
	if err != nil {
		return appdto.EditLock{}, err
	}
	return state, nil
}

func (s *editLockServiceImpl) TakeOver(ctx context.Context, scope editlock.Scope, name string, expectedVersion int) (appdto.EditLock, error) {
	if err := s.open(ctx, scope, true); err != nil {
		return appdto.EditLock{}, err
	}
	holder, err := s.holderOf(ctx, name)
	if err != nil {
		return appdto.EditLock{}, err
	}
	expected, err := versionOfEditLock(expectedVersion)
	if err != nil {
		return appdto.EditLock{}, invalidInput(err)
	}
	lock, found, err := s.locks.Find(ctx, scope)
	if err != nil {
		return appdto.EditLock{}, err
	}
	if !found {
		return appdto.EditLock{}, editlock.ErrEditLockFree
	}
	taken, err := lock.TakeOver(holder, expected, s.now())
	if err != nil {
		return appdto.EditLock{}, err
	}
	if taken == lock {
		return appdto.NewEditLock(lock.Snapshot()), nil
	}
	saved, err := s.locks.Save(ctx, taken)
	if err != nil {
		return appdto.EditLock{}, err
	}
	return appdto.NewEditLock(saved.Snapshot()), nil
}

func (s *editLockServiceImpl) CheckHeld(ctx context.Context, scope editlock.Scope) error {
	var key editlock.Key
	switch editlock.IdentityFrom(ctx) {
	case editlock.IdentitySystem:
		return nil
	case editlock.IdentityClient:
		found, ok := editlock.ClientFrom(ctx)
		if !ok {
			return editlock.ErrNoClient
		}
		key = found
	default:
		return editlock.ErrNoClient
	}
	lock, found, err := s.locks.Find(ctx, scope)
	if err != nil {
		return err
	}
	if !found || !lock.Held() {
		return editlock.NotHeldError(editlock.Snapshot{})
	}
	if lock.Holder().Key() != key {
		return editlock.NotHeldError(lock.Snapshot())
	}
	return s.locks.Touch(ctx, scope, key, s.now())
}

func (s *editLockServiceImpl) Attach(key editlock.Key) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.presence[key]++
}

func (s *editLockServiceImpl) Detach(key editlock.Key) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.presence[key] <= 1 {
		delete(s.presence, key)
		return
	}
	s.presence[key]--
}

// present reports whether a stream of key is open.
func (s *editLockServiceImpl) present(key editlock.Key) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.presence[key] > 0
}

func (s *editLockServiceImpl) ReapOnce(ctx context.Context) error {
	held, err := s.locks.FindAllHeld(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	var errs []error
	for _, lock := range held {
		key := lock.Holder().Key()
		if s.present(key) {
			if err := s.locks.Touch(ctx, lock.Scope(), key, now); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if now.Sub(lock.LastSeen()) < EditLockTTL {
			continue
		}
		expired, err := lock.Expire(now)
		if err != nil {
			continue
		}
		if _, err := s.locks.Save(ctx, expired); err != nil && !errors.Is(err, editlock.ErrEditLockVersionStale) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// holderOf is the holder the request's client declared under name.
func (s *editLockServiceImpl) holderOf(ctx context.Context, name string) (editlock.Holder, error) {
	key, err := s.clientKey(ctx)
	if err != nil {
		return editlock.Holder{}, err
	}
	holder, err := editlock.NewHolder(key, name)
	if err != nil {
		return editlock.Holder{}, invalidInput(err)
	}
	return holder, nil
}

// clientKey is the key the request named.
func (s *editLockServiceImpl) clientKey(ctx context.Context) (editlock.Key, error) {
	key, ok := editlock.ClientFrom(ctx)
	if !ok {
		return editlock.Key{}, editlock.ErrNoClient
	}
	return key, nil
}

func versionOfEditLock(number int) (version.Version[editlock.EditLock], error) {
	v, err := version.New(version.WithNumber[editlock.EditLock](number))
	if err != nil {
		return version.Version[editlock.EditLock]{}, fmt.Errorf("version: %w", err)
	}
	return v, nil
}

// RunEditLockReaper runs ReapOnce every editLockReapInterval until ctx is
// done.
func RunEditLockReaper(ctx context.Context, service EditLockService) {
	ticker := time.NewTicker(editLockReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = service.ReapOnce(ctx)
		}
	}
}
