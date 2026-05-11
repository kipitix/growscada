package restapi

import (
	"encoding/json"
	"net/http"

	"github.com/kipitix/growscada/internal/application"
)

type APIRouter struct {
	serveMux               *http.ServeMux
	tagsHandlers           *TagsHandlers
	indicatorTypesHandlers *IndicatorTypesHandlers
}

func (r APIRouter) ServeMux() *http.ServeMux {
	return r.serveMux
}

func NewRouter(tagService application.TagService, indicatorTypeService application.IndicatorTypeService) *APIRouter {
	router := &APIRouter{}
	router.serveMux = http.NewServeMux()

	router.tagsHandlers = NewTagsHandler(tagService)
	router.serveMux.HandleFunc("GET /api/v1/tags", router.tagsHandlers.GetTags)
	router.serveMux.HandleFunc("GET /api/v1/tags/{id}", router.tagsHandlers.GetTagsByID)
	router.serveMux.HandleFunc("POST /api/v1/tags", router.tagsHandlers.PostTags)
	router.serveMux.HandleFunc("DELETE /api/v1/tags/{id}", router.tagsHandlers.DeleteTagsByID)
	router.serveMux.HandleFunc("PATCH /api/v1/tags/{id}/value", router.tagsHandlers.PatchTagsValue)

	router.indicatorTypesHandlers = NewIndicatorTypesHandler(indicatorTypeService)
	router.serveMux.HandleFunc("GET /api/v1/indicator-types", router.indicatorTypesHandlers.GetIndicatorTypes)
	router.serveMux.HandleFunc("GET /api/v1/indicator-types/{id}", router.indicatorTypesHandlers.GetIndicatorTypesByID)
	router.serveMux.HandleFunc("POST /api/v1/indicator-types", router.indicatorTypesHandlers.PostIndicatorTypes)
	router.serveMux.HandleFunc("PUT /api/v1/indicator-types/{id}", router.indicatorTypesHandlers.PutIndicatorTypesByID)
	router.serveMux.HandleFunc("DELETE /api/v1/indicator-types/{id}", router.indicatorTypesHandlers.DeleteIndicatorTypesByID)

	return router
}

// Helper function for sending a JSON response
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
