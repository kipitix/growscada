package uiutil

import (
	"context"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// SceneOfProject is the key (LoadFor) of the Widgets shown: the Scene SceneID
// of the project whose content Content is the client of.
type SceneOfProject struct {
	Content *apiclient.ProjectClient
	SceneID uuid.UUID
}

// ListWidgets lists the Widgets of the key's Scene, for LoadFor.
func ListWidgets(c context.Context, k SceneOfProject) ([]apiv0.WidgetResponse, error) {
	return k.Content.ListWidgets(c, k.SceneID)
}
