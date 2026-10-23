package restapi

import (
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/brunotest"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// requestBodies maps the pattern of every route that takes a body to the type
// its handler decodes; a POST/PUT/PATCH route without a body maps to nil.
var requestBodies = map[string]any{
	"POST " + apiv0.PathPrefix + "/tags":                                                                        apiv0.CreateTagRequest{},
	"PATCH " + apiv0.PathPrefix + "/tags/" + tagResource.wildcard() + "/value":                                  apiv0.UpdateTagRequest{},
	"POST " + apiv0.PathPrefix + "/widget-types":                                                                apiv0.CreateWidgetTypeRequest{},
	"PUT " + apiv0.PathPrefix + "/widget-types/" + widgetTypeResource.wildcard():                                apiv0.UpdateWidgetTypeRequest{},
	"POST " + apiv0.PathPrefix + "/scenes/" + sceneResource.wildcard() + "/widgets":                             apiv0.CreateWidgetRequest{},
	"PUT " + apiv0.PathPrefix + "/scenes/" + sceneResource.wildcard() + "/widgets/" + widgetResource.wildcard(): apiv0.UpdateWidgetRequest{},
	"POST " + apiv0.PathPrefix + "/scenes":                                                                      apiv0.CreateSceneRequest{},
	"PUT " + apiv0.PathPrefix + "/scenes/" + sceneResource.wildcard():                                           apiv0.UpdateSceneRequest{},
}

// TestBrunoCollection checks that the growscada_server Bruno collection
// keeps up with the API: every request hits a route, every route has a
// request and every body decodes as its handler decodes it.
func TestBrunoCollection(t *testing.T) {
	router := NewRouter(nil, nil, nil, eventbus.NewEventBus(), 1)
	t.Cleanup(router.Close)
	var patterns []string
	for _, rt := range router.routes() {
		patterns = append(patterns, rt.pattern)
	}
	brunotest.Check(t, brunotest.Collection{
		Dir:      brunotest.CollectionDir("growscada_server"),
		Patterns: patterns,
		Bodies:   requestBodies,
		Decode:   decodeStrict,
	})
}
