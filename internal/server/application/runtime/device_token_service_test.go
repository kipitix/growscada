package runtime_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/devicetoken"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// memoryTokens is the devicetoken.Repository in memory: the tests of the
// service need no database.
type memoryTokens struct {
	mu     sync.Mutex
	byHash map[string]devicetoken.Token
	byDev  map[id.ID[device.Device]]string
}

var _ devicetoken.Repository = (*memoryTokens)(nil)

func newMemoryTokens() *memoryTokens {
	return &memoryTokens{
		byHash: make(map[string]devicetoken.Token),
		byDev:  make(map[id.ID[device.Device]]string),
	}
}

func (m *memoryTokens) Insert(_ context.Context, t devicetoken.Token, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byDev[t.DeviceID]; ok {
		return devicetoken.ErrTokenExists
	}
	m.store(t, hash)
	return nil
}

func (m *memoryTokens) Replace(_ context.Context, t devicetoken.Token, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.byDev[t.DeviceID]; ok {
		delete(m.byHash, old)
	}
	m.store(t, hash)
	return nil
}

func (m *memoryTokens) store(t devicetoken.Token, hash string) {
	m.byHash[hash] = t
	m.byDev[t.DeviceID] = hash
}

func (m *memoryTokens) FindByHash(_ context.Context, hash string) (devicetoken.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	found, ok := m.byHash[hash]
	if !ok {
		return devicetoken.Token{}, devicetoken.ErrTokenNotFound
	}
	return found, nil
}

func (m *memoryTokens) FindByDevice(_ context.Context, deviceID id.ID[device.Device]) (devicetoken.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash, ok := m.byDev[deviceID]
	if !ok {
		return devicetoken.Token{}, devicetoken.ErrTokenNotFound
	}
	return m.byHash[hash], nil
}

func (m *memoryTokens) Delete(_ context.Context, deviceID id.ID[device.Device], projectID id.ID[project.Project]) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash, ok := m.byDev[deviceID]
	if !ok || m.byHash[hash].ProjectID != projectID {
		return devicetoken.ErrTokenNotFound
	}
	delete(m.byHash, hash)
	delete(m.byDev, deviceID)
	return nil
}

func (m *memoryTokens) DeleteByProject(_ context.Context, projectID id.ID[project.Project]) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for hash, t := range m.byHash {
		if t.ProjectID == projectID {
			delete(m.byHash, hash)
			delete(m.byDev, t.DeviceID)
		}
	}
	return nil
}

// forget drops the stored token without telling the service: the tests see
// what the cache still answers.
func (m *memoryTokens) forget() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byHash = make(map[string]devicetoken.Token)
	m.byDev = make(map[id.ID[device.Device]]string)
}

func newTokenService() (runtimeapp.DeviceTokenService, *memoryTokens, uuid.UUID, uuid.UUID) {
	repo := newMemoryTokens()
	return runtimeapp.NewDeviceTokenService(repo), repo, uuid.New(), uuid.New()
}

func TestDeviceTokenService_IssueThenAuthenticate(t *testing.T) {
	service, _, projectID, deviceID := newTokenService()
	ctx := context.Background()

	issued, err := service.Issue(ctx, projectID, deviceID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if issued.Secret == "" {
		t.Error("Issue returned no secret")
	}
	if issued.Token.DeviceID != deviceID || issued.Token.ProjectID != projectID {
		t.Errorf("issued token = %+v, want device %s of project %s", issued.Token, deviceID, projectID)
	}
	authenticated, err := service.Authenticate(ctx, issued.Secret)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if authenticated.DeviceID != deviceID || authenticated.ProjectID != projectID {
		t.Errorf("Authenticate = %+v, want device %s of project %s", authenticated, deviceID, projectID)
	}
}

func TestDeviceTokenService_OneTokenPerDevice(t *testing.T) {
	service, _, projectID, deviceID := newTokenService()
	ctx := context.Background()
	if _, err := service.Issue(ctx, projectID, deviceID); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := service.Issue(ctx, projectID, deviceID); !errors.Is(err, devicetoken.ErrTokenExists) {
		t.Errorf("second Issue error = %v, want ErrTokenExists", err)
	}
}

func TestDeviceTokenService_AuthenticateRejectsWhatIsNotADeviceToken(t *testing.T) {
	service, _, _, _ := newTokenService()
	ctx := context.Background()
	for _, secret := range []string{"", "secret", "Bearer gsc_dev_x"} {
		if _, err := service.Authenticate(ctx, secret); !errors.Is(err, devicetoken.ErrNotADeviceToken) {
			t.Errorf("Authenticate(%q) error = %v, want ErrNotADeviceToken", secret, err)
		}
	}
}

func TestDeviceTokenService_AuthenticateOfAnUnknownToken(t *testing.T) {
	service, _, _, _ := newTokenService()
	secret, err := devicetoken.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), secret.String()); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("Authenticate error = %v, want ErrTokenNotFound", err)
	}
}

func TestDeviceTokenService_AuthenticateIsCachedUntilTheTokenChanges(t *testing.T) {
	service, repo, projectID, deviceID := newTokenService()
	ctx := context.Background()
	issued, err := service.Issue(ctx, projectID, deviceID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := service.Authenticate(ctx, issued.Secret); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	repo.forget()
	if _, err := service.Authenticate(ctx, issued.Secret); err != nil {
		t.Fatalf("Authenticate from the cache: %v", err)
	}

	rotated, err := service.Rotate(ctx, projectID, deviceID)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if _, err := service.Authenticate(ctx, issued.Secret); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("the revoked token still authenticates: %v", err)
	}
	if _, err := service.Authenticate(ctx, rotated.Secret); err != nil {
		t.Errorf("the rotated token does not authenticate: %v", err)
	}
}

func TestDeviceTokenService_RotateReplacesTheToken(t *testing.T) {
	service, _, projectID, deviceID := newTokenService()
	ctx := context.Background()
	first, err := service.Issue(ctx, projectID, deviceID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	rotated, err := service.Rotate(ctx, projectID, deviceID)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if rotated.Secret == first.Secret {
		t.Error("rotation returned the same secret")
	}
	if _, err := service.Authenticate(ctx, first.Secret); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("the old secret still authenticates: %v", err)
	}
	if _, err := service.Authenticate(ctx, rotated.Secret); err != nil {
		t.Errorf("the new secret does not authenticate: %v", err)
	}
	// Rotation of a Device that has no token creates one.
	otherDevice := uuid.New()
	if _, err := service.Rotate(ctx, projectID, otherDevice); err != nil {
		t.Fatalf("Rotate without a token: %v", err)
	}
}

func TestDeviceTokenService_RevokeAndFindToken(t *testing.T) {
	service, _, projectID, deviceID := newTokenService()
	ctx := context.Background()
	issued, err := service.Issue(ctx, projectID, deviceID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	found, err := service.FindToken(ctx, projectID, deviceID)
	if err != nil {
		t.Fatalf("FindToken: %v", err)
	}
	if found.Fingerprint != issued.Token.Fingerprint {
		t.Errorf("FindToken fingerprint = %q, want %q", found.Fingerprint, issued.Token.Fingerprint)
	}
	if found.IssuedAt.IsZero() || found.IssuedAt.Location() != time.UTC {
		t.Errorf("FindToken issued_at = %v, want a UTC time", found.IssuedAt)
	}

	if _, err := service.Revoke(ctx, uuid.New(), deviceID); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("Revoke through another project error = %v, want ErrTokenNotFound", err)
	}
	if _, err := service.FindToken(ctx, uuid.New(), deviceID); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("FindToken through another project error = %v, want ErrTokenNotFound", err)
	}
	if _, err := service.Revoke(ctx, projectID, deviceID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := service.Authenticate(ctx, issued.Secret); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("the revoked token still authenticates: %v", err)
	}
	if _, err := service.Revoke(ctx, projectID, deviceID); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("second Revoke error = %v, want ErrTokenNotFound", err)
	}
}

func TestDeviceTokenService_RemoveProjectTokens_ForgetsTheProjectsTokens(t *testing.T) {
	service, repo, projectID, deviceID := newTokenService()
	ctx := context.Background()
	otherProjectID, otherDeviceID := uuid.New(), uuid.New()
	removed, err := service.Issue(ctx, projectID, deviceID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	kept, err := service.Issue(ctx, otherProjectID, otherDeviceID)
	if err != nil {
		t.Fatalf("Issue of the other project: %v", err)
	}
	if _, err := service.Authenticate(ctx, removed.Secret); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if _, err := service.Authenticate(ctx, kept.Secret); err != nil {
		t.Fatalf("Authenticate of the other project: %v", err)
	}

	// The rows are gone without telling the service, as the cascade of a
	// Project's removal does; only the cache can keep them alive.
	repo.forget()
	if err := service.RemoveProjectTokens(ctx, projectID); err != nil {
		t.Fatalf("RemoveProjectTokens: %v", err)
	}
	if _, err := service.Authenticate(ctx, removed.Secret); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("a token of the removed project still authenticates: %v", err)
	}
	if _, err := service.Authenticate(ctx, kept.Secret); err != nil {
		t.Errorf("a token of another project was forgotten too: %v", err)
	}
}

// rotatingTokens rotates the Device's token while a lookup runs: the race of
// an authentication and a rotation, seen from the cache.
type rotatingTokens struct {
	*memoryTokens
	once   sync.Once
	rotate func()
}

func (m *rotatingTokens) FindByHash(ctx context.Context, hash string) (devicetoken.Token, error) {
	found, err := m.memoryTokens.FindByHash(ctx, hash)
	if err == nil {
		// The lookup has the old token; the rotation commits before the
		// authentication writes its result to the cache.
		m.once.Do(m.rotate)
	}
	return found, err
}

func TestDeviceTokenService_RotationDuringAuthentication_IsNotUndoneByTheCache(t *testing.T) {
	repo := newMemoryTokens()
	rotating := &rotatingTokens{memoryTokens: repo}
	service := runtimeapp.NewDeviceTokenService(rotating)
	ctx := context.Background()
	projectID, deviceID := uuid.New(), uuid.New()
	issued, err := service.Issue(ctx, projectID, deviceID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	rotating.rotate = func() {
		if _, err := service.Rotate(ctx, projectID, deviceID); err != nil {
			t.Errorf("Rotate: %v", err)
		}
	}

	// The old token is what the in-flight lookup read, but the rotation
	// cleared the cache afterwards: the authentication must not put it back.
	if _, err := service.Authenticate(ctx, issued.Secret); err != nil {
		t.Fatalf("the in-flight authentication: %v", err)
	}
	if _, err := service.Authenticate(ctx, issued.Secret); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("the rotated-away token still authenticates from the cache: %v", err)
	}
}
