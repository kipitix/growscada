package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// sceneChange changes a Scene as a policy does; the bool reports whether
// anything changed, so that there is something to save.
type sceneChange func(context.Context, scene.Scene) (scene.Scene, bool, error)

// changeEachScene changes every Scene of scenes, each in its own transaction
// (changeScene). A Scene that fails does not stop the others: it returns the
// errors of all that failed.
func changeEachScene(ctx context.Context, repo scene.SceneRepository, scenes []scene.Scene, change sceneChange) error {
	var errs []error
	for _, sc := range scenes {
		if err := changeScene(ctx, repo, sc, change); err != nil {
			errs = append(errs, fmt.Errorf("scene %s: %w", sc.ID(), err))
		}
	}
	return errors.Join(errs...)
}

// changeScene changes the Scene sc and saves it if anything changed. The
// change does not rest on what a client saw, so a Scene changed since it was
// read is reread and changed again rather than reported as a conflict
// (retryOnRace). A Scene deleted meanwhile leaves nothing to change.
func changeScene(ctx context.Context, repo scene.SceneRepository, sc scene.Scene, change sceneChange) error {
	first := true
	err := retryOnRace(scene.ErrSceneConflict, func() error {
		if !first {
			var err error
			if sc, err = repo.FindByID(ctx, sc.ID()); err != nil {
				return fmt.Errorf("cannot reread scene: %w", err)
			}
		}
		first = false
		changed, ok, err := change(ctx, sc)
		if err != nil || !ok {
			return err
		}
		_, err = repo.Save(ctx, changed)
		return err
	})
	if errors.Is(err, scene.ErrSceneNotFound) {
		return nil
	}
	return err
}
