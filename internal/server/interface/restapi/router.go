package restapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/event"
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

// Close releases the router's long-lived resources (currently just the SSE
// event hub's EventBus subscriptions) for graceful shutdown.
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

func NewRouter(tagService application.TagService, widgetTypeService application.WidgetTypeService, sceneService application.SceneService, eventBus event.EventBus, maxSSEClients int) *APIRouter {
	router := &APIRouter{}
	router.serveMux = http.NewServeMux()

	router.tagsHandlers = NewTagsHandler(tagService)
	router.serveMux.HandleFunc("GET "+apiv0.PathPrefix+"/tags", router.tagsHandlers.GetTags)
	router.serveMux.HandleFunc("GET "+apiv0.PathPrefix+"/tags/{id}", router.tagsHandlers.GetTagsByID)
	router.serveMux.HandleFunc("POST "+apiv0.PathPrefix+"/tags", router.tagsHandlers.PostTags)
	router.serveMux.HandleFunc("DELETE "+apiv0.PathPrefix+"/tags/{id}", router.tagsHandlers.DeleteTagsByID)
	router.serveMux.HandleFunc("PATCH "+apiv0.PathPrefix+"/tags/{id}/value", router.tagsHandlers.PatchTagsValue)

	router.widgetTypesHandlers = NewWidgetTypesHandler(widgetTypeService)
	router.serveMux.HandleFunc("GET "+apiv0.PathPrefix+"/widget-types", router.widgetTypesHandlers.GetWidgetTypes)
	router.serveMux.HandleFunc("GET "+apiv0.PathPrefix+"/widget-types/{id}", router.widgetTypesHandlers.GetWidgetTypesByID)
	router.serveMux.HandleFunc("POST "+apiv0.PathPrefix+"/widget-types", router.widgetTypesHandlers.PostWidgetTypes)
	router.serveMux.HandleFunc("PUT "+apiv0.PathPrefix+"/widget-types/{id}", router.widgetTypesHandlers.PutWidgetTypesByID)
	router.serveMux.HandleFunc("DELETE "+apiv0.PathPrefix+"/widget-types/{id}", router.widgetTypesHandlers.DeleteWidgetTypesByID)

	router.widgetsHandlers = NewWidgetsHandler(sceneService)
	router.serveMux.HandleFunc("GET "+apiv0.PathPrefix+"/scenes/{id}/widgets", router.widgetsHandlers.GetWidgetsBySceneID)
	router.serveMux.HandleFunc("POST "+apiv0.PathPrefix+"/scenes/{id}/widgets", router.widgetsHandlers.PostWidgets)
	router.serveMux.HandleFunc("GET "+apiv0.PathPrefix+"/scenes/{sceneId}/widgets/{widgetId}", router.widgetsHandlers.GetWidgetsByID)
	router.serveMux.HandleFunc("PUT "+apiv0.PathPrefix+"/scenes/{sceneId}/widgets/{widgetId}", router.widgetsHandlers.PutWidgetsByID)
	router.serveMux.HandleFunc("DELETE "+apiv0.PathPrefix+"/scenes/{sceneId}/widgets/{widgetId}", router.widgetsHandlers.DeleteWidgetsByID)

	router.scenesHandlers = NewScenesHandler(sceneService)
	router.serveMux.HandleFunc("GET "+apiv0.PathPrefix+"/scenes", router.scenesHandlers.GetScenes)
	router.serveMux.HandleFunc("GET "+apiv0.PathPrefix+"/scenes/{id}", router.scenesHandlers.GetScenesByID)
	router.serveMux.HandleFunc("POST "+apiv0.PathPrefix+"/scenes", router.scenesHandlers.PostScenes)
	router.serveMux.HandleFunc("PUT "+apiv0.PathPrefix+"/scenes/{id}", router.scenesHandlers.PutScenesByID)
	router.serveMux.HandleFunc("DELETE "+apiv0.PathPrefix+"/scenes/{id}", router.scenesHandlers.DeleteScenesByID)

	router.eventsHandlers = NewEventsHandler(eventBus, maxSSEClients)
	router.serveMux.HandleFunc("GET "+apiv0.PathPrefix+"/events", router.eventsHandlers.GetEvents)

	router.serveMux.HandleFunc(preVersioningPathPrefix+"/", preVersioningGone)

	return router
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

// sendInternalError logs the full error and sends a generic 500 to the client.
func sendInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal server error", "path", r.URL.Path, "method", r.Method, "error", err)
	sendJSONResponse(w, http.StatusInternalServerError, NewInternalError("an internal error occurred", r.URL.Path))
}

// sendJSONResponse writes a JSON response with the given status code.
func sendJSONResponse(w http.ResponseWriter, status int, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body) //nolint:errcheck
}
