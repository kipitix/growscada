package operation

import "github.com/kipitix/growscada/internal/server/interface/ui/livescene"

type getScenesResponse struct {
	Scenes []livescene.Scene `json:"scenes"`
}

type getWidgetTypesResponse struct {
	WidgetTypes []livescene.WidgetType `json:"widget_types"`
}

type getWidgetsResponse struct {
	Widgets []livescene.Widget `json:"widgets"`
}

type getTagsResponse struct {
	Tags []livescene.Tag `json:"tags"`
}
