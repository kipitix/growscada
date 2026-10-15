package operation

import (
	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// selectScene keeps the selected Scene if it still exists, otherwise falls
// back to the first Scene (uuid.Nil when there are none).
func selectScene(scenes []apiv0.SceneResponse, selected uuid.UUID) uuid.UUID {
	for _, sc := range scenes {
		if sc.ID == selected {
			return selected
		}
	}
	if len(scenes) > 0 {
		return scenes[0].ID
	}
	return uuid.Nil
}
