package restapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/kipitix/growscada/internal/application"
)

type APIRouter struct {
	serveMux            *http.ServeMux
	tagsHandlers        *TagsHandlers
	widgetTypesHandlers *WidgetTypesHandlers
	widgetsHandlers     *WidgetsHandlers
	scenesHandlers      *ScenesHandlers
}

func (r APIRouter) ServeMux() http.Handler {
	return corsMiddleware(r.serveMux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func NewRouter(tagService application.TagService, widgetTypeService application.WidgetTypeService, widgetService application.WidgetService, sceneService application.SceneService) *APIRouter {
	router := &APIRouter{}
	router.serveMux = http.NewServeMux()

	router.tagsHandlers = NewTagsHandler(tagService)
	router.serveMux.HandleFunc("GET /api/v1/tags", router.tagsHandlers.GetTags)
	router.serveMux.HandleFunc("GET /api/v1/tags/{id}", router.tagsHandlers.GetTagsByID)
	router.serveMux.HandleFunc("POST /api/v1/tags", router.tagsHandlers.PostTags)
	router.serveMux.HandleFunc("DELETE /api/v1/tags/{id}", router.tagsHandlers.DeleteTagsByID)
	router.serveMux.HandleFunc("PATCH /api/v1/tags/{id}/value", router.tagsHandlers.PatchTagsValue)

	router.widgetTypesHandlers = NewWidgetTypesHandler(widgetTypeService)
	router.serveMux.HandleFunc("GET /api/v1/widget-types", router.widgetTypesHandlers.GetWidgetTypes)
	router.serveMux.HandleFunc("GET /api/v1/widget-types/{id}", router.widgetTypesHandlers.GetWidgetTypesByID)
	router.serveMux.HandleFunc("POST /api/v1/widget-types", router.widgetTypesHandlers.PostWidgetTypes)
	router.serveMux.HandleFunc("PUT /api/v1/widget-types/{id}", router.widgetTypesHandlers.PutWidgetTypesByID)
	router.serveMux.HandleFunc("DELETE /api/v1/widget-types/{id}", router.widgetTypesHandlers.DeleteWidgetTypesByID)

	router.widgetsHandlers = NewWidgetsHandler(widgetService)
	router.serveMux.HandleFunc("GET /api/v1/widgets", router.widgetsHandlers.GetWidgets)
	router.serveMux.HandleFunc("GET /api/v1/widgets/{id}", router.widgetsHandlers.GetWidgetsByID)
	router.serveMux.HandleFunc("POST /api/v1/widgets", router.widgetsHandlers.PostWidgets)
	router.serveMux.HandleFunc("PUT /api/v1/widgets/{id}", router.widgetsHandlers.PutWidgetsByID)
	router.serveMux.HandleFunc("DELETE /api/v1/widgets/{id}", router.widgetsHandlers.DeleteWidgetsByID)

	router.scenesHandlers = NewScenesHandler(sceneService)
	router.serveMux.HandleFunc("GET /api/v1/scenes", router.scenesHandlers.GetScenes)
	router.serveMux.HandleFunc("GET /api/v1/scenes/{id}", router.scenesHandlers.GetScenesByID)
	router.serveMux.HandleFunc("POST /api/v1/scenes", router.scenesHandlers.PostScenes)
	router.serveMux.HandleFunc("PUT /api/v1/scenes/{id}", router.scenesHandlers.PutScenesByID)
	router.serveMux.HandleFunc("DELETE /api/v1/scenes/{id}", router.scenesHandlers.DeleteScenesByID)

	return router
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
