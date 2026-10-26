package restapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

type APIRouter struct {
	serveMux            *http.ServeMux
	projectsHandlers    *ProjectsHandlers
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

// NewRouter returns the server API over the services of both roles: the
// Engineering node's, and the Runtime node's tagStateService, which writes
// the tags' states (ADR 0011).
func NewRouter(projectService application.ProjectService, tagService application.TagService, tagStateService runtimeapp.TagStateService, widgetTypeService application.WidgetTypeService, sceneService application.SceneService, eventBus eventbus.EventBus, maxSSEClients int) *APIRouter {
	router := &APIRouter{
		serveMux:            http.NewServeMux(),
		projectsHandlers:    NewProjectsHandler(projectService),
		tagsHandlers:        NewTagsHandler(tagService, tagStateService),
		widgetTypesHandlers: NewWidgetTypesHandler(widgetTypeService),
		widgetsHandlers:     NewWidgetsHandler(sceneService),
		scenesHandlers:      NewScenesHandler(sceneService),
		eventsHandlers:      NewEventsHandler(eventBus, projectService, maxSSEClients),
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
// The content of a project lives under its path, pp (ADR 0006).
func (r *APIRouter) routes() []route {
	const p = apiv0.PathPrefix
	pp := p + apiv0.ProjectPath(projectResource.wildcard())
	projects, tags, widgetTypes, widgets, scenes := r.projectsHandlers, r.tagsHandlers, r.widgetTypesHandlers, r.widgetsHandlers, r.scenesHandlers
	return []route{
		{"GET " + p + "/projects", projects.GetProjects},
		{"GET " + pp, projects.GetProjectsByID},
		{"POST " + p + "/projects", projects.PostProjects},
		{"PUT " + pp, projects.PutProjectsByID},
		{"DELETE " + pp, projects.DeleteProjectsByID},

		{"GET " + pp + "/tags", tags.GetTags},
		{"GET " + pp + "/tags/" + tagResource.wildcard(), tags.GetTagsByID},
		{"POST " + pp + "/tags", tags.PostTags},
		{"DELETE " + pp + "/tags/" + tagResource.wildcard(), tags.DeleteTagsByID},
		{"PATCH " + pp + "/tags/" + tagResource.wildcard() + "/value", tags.PatchTagsValue},

		{"GET " + p + "/widget-types", widgetTypes.GetWidgetTypes},
		{"GET " + p + "/widget-types/" + widgetTypeResource.wildcard(), widgetTypes.GetWidgetTypesByID},
		{"POST " + p + "/widget-types", widgetTypes.PostWidgetTypes},
		{"PUT " + p + "/widget-types/" + widgetTypeResource.wildcard(), widgetTypes.PutWidgetTypesByID},
		{"DELETE " + p + "/widget-types/" + widgetTypeResource.wildcard(), widgetTypes.DeleteWidgetTypesByID},

		{"GET " + pp + "/scenes/" + sceneResource.wildcard() + "/widgets", widgets.GetWidgetsBySceneID},
		{"POST " + pp + "/scenes/" + sceneResource.wildcard() + "/widgets", widgets.PostWidgets},
		{"GET " + pp + "/scenes/" + sceneResource.wildcard() + "/widgets/" + widgetResource.wildcard(), widgets.GetWidgetsByID},
		{"PUT " + pp + "/scenes/" + sceneResource.wildcard() + "/widgets/" + widgetResource.wildcard(), widgets.PutWidgetsByID},
		{"DELETE " + pp + "/scenes/" + sceneResource.wildcard() + "/widgets/" + widgetResource.wildcard(), widgets.DeleteWidgetsByID},

		{"GET " + pp + "/scenes", scenes.GetScenes},
		{"GET " + pp + "/scenes/" + sceneResource.wildcard(), scenes.GetScenesByID},
		{"POST " + pp + "/scenes", scenes.PostScenes},
		{"PUT " + pp + "/scenes/" + sceneResource.wildcard(), scenes.PutScenesByID},
		{"DELETE " + pp + "/scenes/" + sceneResource.wildcard(), scenes.DeleteScenesByID},

		{"GET " + p + "/events", r.eventsHandlers.GetEvents},
		{"GET " + pp + "/events", r.eventsHandlers.GetProjectEvents},

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
	sendProblem(w, NewGone(detail, r.URL.Path))
}

// sendJSONResponse writes a successful JSON response with the given status
// code. Errors go through sendProblem.
func sendJSONResponse(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, "application/json", data)
}

// sendProblem writes Problem Details as application/problem+json (RFC 7807),
// with the status the problem states.
func sendProblem(w http.ResponseWriter, p *apiv0.Problem) {
	writeJSON(w, p.Status, "application/problem+json", p)
}

func writeJSON(w http.ResponseWriter, status int, contentType string, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	w.Write(body) //nolint:errcheck
}
