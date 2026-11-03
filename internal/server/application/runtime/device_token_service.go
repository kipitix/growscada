package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/devicetoken"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// DeviceToken is a Device Token as the Runtime node hands it out: the Device
// it acts for, that Device's Project, its fingerprint and when it was
// issued — everything but the secret.
type DeviceToken struct {
	DeviceID    uuid.UUID
	ProjectID   uuid.UUID
	Fingerprint string
	IssuedAt    time.Time
}

// IssuedDeviceToken is a token just issued or rotated: Secret is shown to the
// caller once and never stored.
type IssuedDeviceToken struct {
	Token  DeviceToken
	Secret string
}

// DeviceTokenService is the Runtime node's service of Device Tokens (ADR
// 0004, amendment 2026-11-01): one token per Device, issued here and never
// part of a Revision. Every method takes the Project first; the Project of a
// token is the one its Device belongs to.
type DeviceTokenService interface {
	// Authenticate resolves a bearer to the token behind it: the Device it
	// acts for and that Device's Project. A token that is unknown or revoked
	// is devicetoken.ErrTokenNotFound; a bearer without the Device Token
	// prefix is devicetoken.ErrNotADeviceToken. A resolved token is kept in
	// memory until it is rotated or revoked.
	Authenticate(ctx context.Context, secret string) (DeviceToken, error)
	// Issue gives the Device a token, which it had none of
	// (devicetoken.ErrTokenExists otherwise).
	Issue(ctx context.Context, projectID, deviceID uuid.UUID) (IssuedDeviceToken, error)
	// Rotate replaces the Device's token, revoking the old one; a Device
	// without one gets it.
	Rotate(ctx context.Context, projectID, deviceID uuid.UUID) (IssuedDeviceToken, error)
	// Revoke removes the Device's token
	// (devicetoken.ErrTokenNotFound if it has none).
	Revoke(ctx context.Context, projectID, deviceID uuid.UUID) (DeviceToken, error)
	// FindToken returns the metadata of the Device's token, its secret never
	// included (devicetoken.ErrTokenNotFound if it has none).
	FindToken(ctx context.Context, projectID, deviceID uuid.UUID) (DeviceToken, error)
	// RemoveProjectTokens removes every token of the Project and forgets the
	// ones already resolved in memory. A Project without tokens is not an
	// error. The cleanup of a deleted Project calls it: its token rows go
	// with its row, and a token kept in memory would authenticate a Project
	// that no longer exists.
	RemoveProjectTokens(ctx context.Context, projectID uuid.UUID) error
}

type deviceTokenServiceImpl struct {
	tokens devicetoken.Repository
	// cache keeps the tokens already resolved by hash: a token changes when
	// it is rotated, revoked or removed with its Project, and every one of
	// them clears it. It makes authentication — the check of every Gateway
	// request — one map read on the hot path.
	cache *tokenCache
	now   func() time.Time
}

var _ DeviceTokenService = (*deviceTokenServiceImpl)(nil)

// NewDeviceTokenService returns the service of the Device Tokens.
func NewDeviceTokenService(tokens devicetoken.Repository) DeviceTokenService {
	return &deviceTokenServiceImpl{tokens: tokens, cache: newTokenCache(), now: time.Now}
}

func (s deviceTokenServiceImpl) Authenticate(ctx context.Context, secret string) (DeviceToken, error) {
	parsed, err := devicetoken.ParseSecret(secret)
	if err != nil {
		return DeviceToken{}, err
	}
	hash := parsed.Hash()
	if found, ok := s.cache.get(hash); ok {
		return newDeviceToken(found), nil
	}
	// The generation is read before the lookup: a rotation, a revocation or a
	// Project's removal between the lookup and the cache write bumps it, and
	// the write is dropped instead of caching a token that just changed.
	generation := s.cache.currentGeneration()
	found, err := s.tokens.FindByHash(ctx, hash)
	if err != nil {
		return DeviceToken{}, fmt.Errorf("cannot authenticate device token: %w", err)
	}
	s.cache.put(hash, found, generation)
	return newDeviceToken(found), nil
}

func (s deviceTokenServiceImpl) Issue(ctx context.Context, rawProjectID, rawDeviceID uuid.UUID) (IssuedDeviceToken, error) {
	token, secret, err := s.newToken(rawProjectID, rawDeviceID)
	if err != nil {
		return IssuedDeviceToken{}, err
	}
	if err := s.tokens.Insert(ctx, token, secret.Hash()); err != nil {
		return IssuedDeviceToken{}, fmt.Errorf("cannot issue device token: %w", err)
	}
	return issuedDeviceToken(token, secret), nil
}

func (s deviceTokenServiceImpl) Rotate(ctx context.Context, rawProjectID, rawDeviceID uuid.UUID) (IssuedDeviceToken, error) {
	token, secret, err := s.newToken(rawProjectID, rawDeviceID)
	if err != nil {
		return IssuedDeviceToken{}, err
	}
	if err := s.tokens.Replace(ctx, token, secret.Hash()); err != nil {
		return IssuedDeviceToken{}, fmt.Errorf("cannot rotate device token: %w", err)
	}
	s.cache.clear()
	return issuedDeviceToken(token, secret), nil
}

func (s deviceTokenServiceImpl) Revoke(ctx context.Context, rawProjectID, rawDeviceID uuid.UUID) (DeviceToken, error) {
	projectID := projectIDOf(rawProjectID)
	deviceID := deviceIDOf(rawDeviceID)
	found, err := s.tokens.FindByDevice(ctx, deviceID)
	if err != nil {
		return DeviceToken{}, fmt.Errorf("cannot revoke device token: %w", err)
	}
	if found.ProjectID != projectID {
		return DeviceToken{}, fmt.Errorf("cannot revoke device token: %w", devicetoken.ErrTokenNotFound)
	}
	if err := s.tokens.Delete(ctx, deviceID, projectID); err != nil {
		return DeviceToken{}, fmt.Errorf("cannot revoke device token: %w", err)
	}
	s.cache.clear()
	return newDeviceToken(found), nil
}

func (s deviceTokenServiceImpl) FindToken(ctx context.Context, rawProjectID, rawDeviceID uuid.UUID) (DeviceToken, error) {
	found, err := s.tokens.FindByDevice(ctx, deviceIDOf(rawDeviceID))
	if err != nil {
		return DeviceToken{}, fmt.Errorf("cannot find device token: %w", err)
	}
	if found.ProjectID != projectIDOf(rawProjectID) {
		return DeviceToken{}, fmt.Errorf("cannot find device token: %w", devicetoken.ErrTokenNotFound)
	}
	return newDeviceToken(found), nil
}

func (s deviceTokenServiceImpl) RemoveProjectTokens(ctx context.Context, rawProjectID uuid.UUID) error {
	projectID := projectIDOf(rawProjectID)
	if err := s.tokens.DeleteByProject(ctx, projectID); err != nil {
		return fmt.Errorf("cannot remove the device tokens: %w", err)
	}
	s.cache.forgetProject(projectID)
	return nil
}

// newToken makes the record of a fresh token and its secret.
func (s deviceTokenServiceImpl) newToken(rawProjectID, rawDeviceID uuid.UUID) (devicetoken.Token, devicetoken.Secret, error) {
	secret, err := devicetoken.NewSecret()
	if err != nil {
		return devicetoken.Token{}, devicetoken.Secret{}, err
	}
	token := devicetoken.NewToken(deviceIDOf(rawDeviceID), projectIDOf(rawProjectID), secret, s.now())
	return token, secret, nil
}

func issuedDeviceToken(token devicetoken.Token, secret devicetoken.Secret) IssuedDeviceToken {
	return IssuedDeviceToken{Token: newDeviceToken(token), Secret: secret.String()}
}

func newDeviceToken(t devicetoken.Token) DeviceToken {
	return DeviceToken{
		DeviceID:    t.DeviceID.UUID(),
		ProjectID:   t.ProjectID.UUID(),
		Fingerprint: t.Fingerprint,
		IssuedAt:    t.IssuedAt.UTC(),
	}
}

func deviceIDOf(raw uuid.UUID) id.ID[device.Device] {
	return id.NewID(id.IDWithUUID[device.Device](raw))
}

// tokenCache keeps the tokens resolved by authentication, by their hash.
type tokenCache struct {
	mu         sync.RWMutex
	byHash     map[string]devicetoken.Token
	generation uint64
}

func newTokenCache() *tokenCache {
	return &tokenCache{byHash: make(map[string]devicetoken.Token)}
}

func (c *tokenCache) get(hash string) (devicetoken.Token, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	found, ok := c.byHash[hash]
	return found, ok
}

// currentGeneration is the number of the changes the cache has seen. An
// authentication that started before a change must not write its result
// after it.
func (c *tokenCache) currentGeneration() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.generation
}

// put stores the token unless the cache changed since generation was read: a
// token resolved before a rotation, a revocation or a Project's removal is
// never cached after it.
func (c *tokenCache) put(hash string, token devicetoken.Token, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation {
		return
	}
	c.byHash[hash] = token
}

// clear forgets every resolved token: what a rotation or a revocation
// changes.
func (c *tokenCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.byHash)
	c.generation++
}

// forgetProject forgets the resolved tokens of the Project: the ones whose
// rows the removal of the Project took with it.
func (c *tokenCache) forgetProject(projectID id.ID[project.Project]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for hash, token := range c.byHash {
		if token.ProjectID == projectID {
			delete(c.byHash, hash)
		}
	}
	c.generation++
}
