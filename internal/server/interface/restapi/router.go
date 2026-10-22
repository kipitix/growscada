package restapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

type APIRouter struct {
	serveMux            *http.ServeMux
	tagsHandlers        *TagsHandlers
	widgetTypesHandlers *WidgetTypesHandlers
	widgetsHandlers     *WidgetsHandlers
	scenesHandlers      *ScenesHandlers
	eventsHandlers      *EventsHandlers
}

func (r APIRouter) ServeMux() http.Handler {
	return schemaVersionMiddleware(corsMiddleware(r.serveMux))
}

// Close prepares the router for graceful shutdown: the SSE event hub leaves
// the EventBus and ends every open event stream, and later streams get 503.
// http.Server.Shutdown waits for those streams, so Close must not wait for
// Shutdown to return.
func (r APIRouter) Close() {
	r.eventsHandlers.Close()
}

// corsMaxAge is how long, in seconds, a browser may cache a CORS preflight.
const corsMaxAge = "7200"

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+contract.SchemaVersionHeader)
		w.Header().Set("Access-Control-Expose-Headers", contract.SchemaVersionHeader)
		// Lets the browser reuse a preflight instead of repeating it before every
		// POST/PUT/PATCH/DELETE; browsers cap the value (Chromium at 2 hours).
		w.Header().Set("Access-Control-Max-Age", corsMaxAge)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func NewRouter(tagService application.TagService, widgetTypeService application.WidgetTypeService, sceneService application.SceneService, eventBus eventbus.EventBus, maxSSEClients int) *APIRouter {
	router := &APIRouter{
		serveMux:            http.NewServeMux(),
		tagsHandlers:        NewTagsHandler(tagService),
		widgetTypesHandlers: NewWidgetTypesHandler(widgetTypeService),
		widgetsHandlers:     NewWidgetsHandler(sceneService),
		scenesHandlers:      NewScenesHandler(sceneService),
		eventsHandlers:      NewEventsHandler(eventBus, maxSSEClients),
	}
	for _, rt := range router.routes() {
		router.serveMux.HandleFunc(rt.pattern, rt.handler)
	}
	return router
}

// route is one ServeMux pattern and its handler.
type route struct {
	pattern string
	handler http.HandlerFunc
}

// routes is every route of the API. The Bruno collection check
// (bruno_collection_internal_test.go) matches the collection against it.
func (r *APIRouter) routes() []route {
	const p = apiv0.PathPrefix
	tags, widgetTypes, widgets, scenes := r.tagsHandlers, r.widgetTypesHandlers, r.widgetsHandlers, r.scenesHandlers
	return []route{
		{"GET " + p + "/tags", tags.GetTags},
		{"GET " + p + "/tags/" + tagResource.wildcard(), tags.GetTagsByID},
		{"POST " + p + "/tags", tags.PostTags},
		{"DELETE " + p + "/tags/" + tagResource.wildcard(), tags.DeleteTagsByID},
		{"PATCH " + p + "/tags/" + tagResource.wildcard() + "/value", tags.PatchTagsValue},

		{"GET " + p + "/widget-types", widgetTypes.GetWidgetTypes},
		{"GET " + p + "/widget-types/" + widgetTypeResource.wildcard(), widgetTypes.GetWidgetTypesByID},
		{"POST " + p + "/widget-types", widgetTypes.PostWidgetTypes},
		{"PUT " + p + "/widget-types/" + widgetTypeResource.wildcard(), widgetTypes.PutWidgetTypesByID},
		{"DELETE " + p + "/widget-types/" + widgetTypeResource.wildcard(), widgetTypes.DeleteWidgetTypesByID},

		{"GET " + p + "/scenes/" + sceneResource.wildcard() + "/widgets", widgets.GetWidgetsBySceneID},
		{"POST " + p + "/scenes/" + sceneResource.wildcard() + "/widgets", widgets.PostWidgets},
		{"GET " + p + "/scenes/" + sceneResource.wildcard() + "/widgets/" + widgetResource.wildcard(), widgets.GetWidgetsByID},
		{"PUT " + p + "/scenes/" + sceneResource.wildcard() + "/widgets/" + widgetResource.wildcard(), widgets.PutWidgetsByID},
		{"DELETE " + p + "/scenes/" + sceneResource.wildcard() + "/widgets/" + widgetResource.wildcard(), widgets.DeleteWidgetsByID},

		{"GET " + p + "/scenes", scenes.GetScenes},
		{"GET " + p + "/scenes/" + sceneResource.wildcard(), scenes.GetScenesByID},
		{"POST " + p + "/scenes", scenes.PostScenes},
		{"PUT " + p + "/scenes/" + sceneResource.wildcard(), scenes.PutScenesByID},
		{"DELETE " + p + "/scenes/" + sceneResource.wildcard(), scenes.DeleteScenesByID},

		{"GET " + p + "/events", r.eventsHandlers.GetEvents},

		{preVersioningPathPrefix + "/", preVersioningGone},
	}
}

// preVersioningPathPrefix is where the API lived before ADR 0005: the shapes
// of today's v0. A client still calling it is told where the API moved rather
// than getting a bare 404. Remove it when the API reaches 1.0, where the path
// takes its real meaning.
const preVersioningPathPrefix = "/api/v1"

func preVersioningGone(w http.ResponseWriter, r *http.Request) {
	detail := fmt.Sprintf("%s is the server API from before contract versioning: use %s",
		preVersioningPathPrefix, apiv0.PathPrefix)
	sendJSONResponse(w, http.StatusGone, NewGone(detail, r.URL.Path))
}

// sendJSONResponse writes a JSON response with the given status code. Problem
// Details go as application/problem+json (RFC 7807).
func sendJSONResponse(w http.ResponseWriter, status int, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	contentType := "application/json"
	if _, ok := data.(*apiv0.Problem); ok {
		contentType = "application/problem+json"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	w.Write(body) //nolint:errcheck
}
