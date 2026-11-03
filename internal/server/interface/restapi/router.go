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
	serveMux             *http.ServeMux
	projectsHandlers     *ProjectsHandlers
	librariesHandlers    *LibrariesHandlers
	libraryPinsHandlers  *LibraryPinsHandlers
	tagsHandlers         *TagsHandlers
	widgetTypesHandlers  *WidgetTypesHandlers
	widgetsHandlers      *WidgetsHandlers
	scenesHandlers       *ScenesHandlers
	devicesHandlers      *DevicesHandlers
	environmentsHandlers *EnvironmentsHandlers
	deviceTokensHandlers *DeviceTokensHandlers
	deviceConfigHandlers *DeviceConfigHandlers
	deviceWhoAmIHandlers *DeviceIdentityHandlers
	eventsHandlers       *EventsHandlers
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, "+contract.SchemaVersionHeader)
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

// Services are the services the server API serves: the Engineering node's,
// and the Runtime node's TagStates, which writes the tags' states,
// DeviceTokens, which authenticates Devices and manages their tokens, and
// DeviceConfig, which reads the running Revision's copies (ADR 0011). A
// Device token service that is nil is a server without the Runtime node:
// token management answers 404.
type Services struct {
	Projects     application.ProjectService
	Tags         application.TagService
	TagStates    runtimeapp.TagStateService
	Libraries    application.LibraryService
	WidgetTypes  application.WidgetTypeService
	LibraryPins  application.LibraryPinService
	Scenes       application.SceneService
	Devices      application.DeviceService
	Environments application.EnvironmentService
	DeviceConfig runtimeapp.DeviceConfigService
	DeviceTokens runtimeapp.DeviceTokenService
}

// NewRouter returns the server API over the services of both roles.
func NewRouter(services Services, eventBus eventbus.EventBus, maxSSEClients int) *APIRouter {
	projectService, tagService, tagStateService := services.Projects, services.Tags, services.TagStates
	widgetTypeService, sceneService := services.WidgetTypes, services.Scenes
	router := &APIRouter{
		serveMux:             http.NewServeMux(),
		projectsHandlers:     NewProjectsHandler(projectService),
		librariesHandlers:    NewLibrariesHandler(services.Libraries),
		libraryPinsHandlers:  NewLibraryPinsHandler(services.LibraryPins),
		tagsHandlers:         NewTagsHandler(tagService, tagStateService, services.DeviceTokens),
		widgetTypesHandlers:  NewWidgetTypesHandler(widgetTypeService),
		widgetsHandlers:      NewWidgetsHandler(sceneService),
		scenesHandlers:       NewScenesHandler(sceneService),
		devicesHandlers:      NewDevicesHandler(services.Devices),
		environmentsHandlers: NewEnvironmentsHandler(services.Environments),
		deviceTokensHandlers: NewDeviceTokensHandler(services.DeviceTokens, services.Devices),
		deviceConfigHandlers: NewDeviceConfigHandler(services.DeviceConfig, services.DeviceTokens),
		deviceWhoAmIHandlers: NewDeviceIdentityHandler(services.DeviceTokens),
		eventsHandlers:       NewEventsHandler(eventBus, projectService, maxSSEClients, WithDeviceTokens(services.DeviceTokens)),
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
	lp := p + apiv0.LibraryPath(libraryResource.wildcard())
	projects, tags, widgetTypes, widgets, scenes := r.projectsHandlers, r.tagsHandlers, r.widgetTypesHandlers, r.widgetsHandlers, r.scenesHandlers
	libraries, pins := r.librariesHandlers, r.libraryPinsHandlers
	devices, environments := r.devicesHandlers, r.environmentsHandlers
	tokens, config := r.deviceTokensHandlers, r.deviceConfigHandlers
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

		{"GET " + p + "/libraries", libraries.GetLibraries},
		{"GET " + lp, libraries.GetLibrariesByID},
		{"POST " + p + "/libraries", libraries.PostLibraries},
		{"PUT " + lp, libraries.PutLibrariesByID},
		{"DELETE " + lp, libraries.DeleteLibrariesByID},
		{"POST " + lp + "/releases", libraries.PostReleases},
		{"GET " + lp + "/releases", libraries.GetReleases},
		{"GET " + lp + "/releases/" + releaseResource.wildcard(), libraries.GetReleasesByNumber},
		{"POST " + lp + "/discard", libraries.PostDiscard},

		{"GET " + lp + "/widget-types", widgetTypes.GetWidgetTypes},
		{"GET " + lp + "/widget-types/" + widgetTypeResource.wildcard(), widgetTypes.GetWidgetTypesByID},
		{"POST " + lp + "/widget-types", widgetTypes.PostWidgetTypes},
		{"PUT " + lp + "/widget-types/" + widgetTypeResource.wildcard(), widgetTypes.PutWidgetTypesByID},
		{"DELETE " + lp + "/widget-types/" + widgetTypeResource.wildcard(), widgetTypes.DeleteWidgetTypesByID},

		{"GET " + pp + "/library-pins", pins.GetLibraryPins},
		{"PUT " + pp + "/library-pins/" + libraryPinResource.wildcard(), pins.PutLibraryPin},
		{"DELETE " + pp + "/library-pins/" + libraryPinResource.wildcard(), pins.DeleteLibraryPin},
		{"GET " + pp + "/library-pins/" + libraryPinResource.wildcard() + "/impact", pins.GetLibraryPinImpact},
		{"GET " + pp + "/widget-types", pins.GetPinnedWidgetTypes},

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

		{"GET " + pp + "/devices", devices.GetDevices},
		{"GET " + pp + "/devices/" + deviceResource.wildcard(), devices.GetDevicesByID},
		{"POST " + pp + "/devices", devices.PostDevices},
		{"PUT " + pp + "/devices/" + deviceResource.wildcard(), devices.PutDevicesByID},
		{"DELETE " + pp + "/devices/" + deviceResource.wildcard(), devices.DeleteDevicesByID},
		{"GET " + pp + "/devices/" + deviceResource.wildcard() + "/token", tokens.GetDeviceToken},
		{"POST " + pp + "/devices/" + deviceResource.wildcard() + "/token", tokens.PostDeviceToken},
		{"DELETE " + pp + "/devices/" + deviceResource.wildcard() + "/token", tokens.DeleteDeviceToken},
		{"POST " + pp + "/devices/" + deviceResource.wildcard() + "/token/rotate", tokens.PostDeviceTokenRotate},

		{"GET " + pp + "/environments", environments.GetEnvironments},
		{"GET " + pp + "/environments/" + environmentResource.wildcard(), environments.GetEnvironmentsByID},
		{"POST " + pp + "/environments", environments.PostEnvironments},
		{"PUT " + pp + "/environments/" + environmentResource.wildcard(), environments.PutEnvironmentsByID},
		{"DELETE " + pp + "/environments/" + environmentResource.wildcard(), environments.DeleteEnvironmentsByID},
		{"GET " + pp + "/environments/" + environmentResource.wildcard() + "/devices", environments.GetEnvironmentDevices},
		{"PUT " + pp + "/environments/" + environmentResource.wildcard() + "/devices/" + deviceResource.wildcard(), environments.PutEnvironmentDevice},
		{"DELETE " + pp + "/environments/" + environmentResource.wildcard() + "/devices/" + deviceResource.wildcard(), environments.DeleteEnvironmentDevice},

		{"GET " + p + "/device", r.deviceWhoAmIHandlers.GetDeviceIdentity},
		{"GET " + p + "/device/config", config.GetDeviceConfig},
		{"GET " + p + "/device/events", r.eventsHandlers.GetDeviceEvents},

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
	if p.Status == http.StatusUnauthorized {
		// RFC 9110: a 401 carries at least one challenge.
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
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
