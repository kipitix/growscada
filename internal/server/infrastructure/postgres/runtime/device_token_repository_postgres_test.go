package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/devicetoken"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
)

// newTokenProject cleans the tokens and returns a new live project.
func newTokenProject(t *testing.T) id.ID[project.Project] {
	t.Helper()
	if _, err := postgres.DB(t).ExecContext(context.Background(), `DELETE FROM runtime.device_tokens`); err != nil {
		t.Fatalf("clean tokens: %v", err)
	}
	projectID, _ := newProject(t)
	return projectID
}

// newToken returns the record of a token of a fresh Device of the project.
func newToken(t *testing.T, projectID id.ID[project.Project]) (devicetoken.Token, string) {
	t.Helper()
	secret, err := devicetoken.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	token := devicetoken.NewToken(id.NewID[device.Device](), projectID, secret, time.Now())
	return token, secret.Hash()
}

func TestDeviceToken_InsertFindAndRefuseASecond(t *testing.T) {
	projectID := newTokenProject(t)
	repo := runtimepg.NewDeviceTokenRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	token, hash := newToken(t, projectID)

	if err := repo.Insert(ctx, token, hash); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	byHash, err := repo.FindByHash(ctx, hash)
	if err != nil {
		t.Fatalf("FindByHash: %v", err)
	}
	if byHash.DeviceID != token.DeviceID || byHash.ProjectID != projectID || byHash.Fingerprint != token.Fingerprint {
		t.Errorf("FindByHash = %+v, want %+v", byHash, token)
	}
	byDevice, err := repo.FindByDevice(ctx, token.DeviceID)
	if err != nil {
		t.Fatalf("FindByDevice: %v", err)
	}
	if byDevice.Fingerprint != token.Fingerprint {
		t.Errorf("FindByDevice = %+v, want the token of %s", byDevice, token.DeviceID)
	}
	if err := repo.Insert(ctx, token, hash); !errors.Is(err, devicetoken.ErrTokenExists) {
		t.Errorf("second Insert error = %v, want ErrTokenExists", err)
	}
}

func TestDeviceToken_ReplaceRotatesTheToken(t *testing.T) {
	projectID := newTokenProject(t)
	repo := runtimepg.NewDeviceTokenRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	oldToken, oldHash := newToken(t, projectID)
	if err := repo.Insert(ctx, oldToken, oldHash); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	rotated, newHash := newToken(t, projectID)
	rotated.DeviceID = oldToken.DeviceID
	if err := repo.Replace(ctx, rotated, newHash); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if _, err := repo.FindByHash(ctx, oldHash); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("the old hash is still stored: %v", err)
	}
	found, err := repo.FindByHash(ctx, newHash)
	if err != nil {
		t.Fatalf("FindByHash of the new token: %v", err)
	}
	if found.Fingerprint != rotated.Fingerprint {
		t.Errorf("rotated token fingerprint = %q, want %q", found.Fingerprint, rotated.Fingerprint)
	}
}

func TestDeviceToken_ReplaceInsertsWhenTheDeviceHasNone(t *testing.T) {
	projectID := newTokenProject(t)
	repo := runtimepg.NewDeviceTokenRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	token, hash := newToken(t, projectID)

	if err := repo.Replace(ctx, token, hash); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if _, err := repo.FindByDevice(ctx, token.DeviceID); err != nil {
		t.Errorf("FindByDevice after Replace: %v", err)
	}
}

func TestDeviceToken_DeleteOfTheProjectOnly(t *testing.T) {
	projectID := newTokenProject(t)
	repo := runtimepg.NewDeviceTokenRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	token, hash := newToken(t, projectID)
	if err := repo.Insert(ctx, token, hash); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	otherProject := id.NewID[project.Project]()
	if err := repo.Delete(ctx, token.DeviceID, otherProject); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("Delete of another project error = %v, want ErrTokenNotFound", err)
	}
	if _, err := repo.FindByHash(ctx, hash); err != nil {
		t.Fatalf("the token of another project was removed: %v", err)
	}
	if err := repo.Delete(ctx, token.DeviceID, projectID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByHash(ctx, hash); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("FindByHash after Delete error = %v, want ErrTokenNotFound", err)
	}
	if err := repo.Delete(ctx, token.DeviceID, projectID); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("second Delete error = %v, want ErrTokenNotFound", err)
	}
}

func TestDeviceToken_GoesWithItsProject(t *testing.T) {
	projectID := newTokenProject(t)
	repo := runtimepg.NewDeviceTokenRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	token, hash := newToken(t, projectID)
	if err := repo.Insert(ctx, token, hash); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if _, err := postgres.DB(t).ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if _, err := repo.FindByHash(ctx, hash); !errors.Is(err, devicetoken.ErrTokenNotFound) {
		t.Errorf("the token outlived its project: %v", err)
	}
}
