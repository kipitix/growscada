package restapi

import (
	"encoding/json"
	"net/http"

	"gitverse.ru/kipitix/growscada/internal/application"
)

type APIRouter struct {
	serveMux     *http.ServeMux
	tagsHandlers *TagsHandlers
}

func (r APIRouter) ServeMux() *http.ServeMux {
	return r.serveMux
}

func NewRouter(tagService application.TagService) *APIRouter {
	// Create router — container for handlers
	router := &APIRouter{}

	// Create mux and register handlers
	router.serveMux = http.NewServeMux()

	// Create handlers — wrappers over services
	router.tagsHandlers = NewTagsHandler(tagService)

	// Register handlers
	router.serveMux.HandleFunc("GET /api/v1/tags", router.tagsHandlers.GetTags)
	router.serveMux.HandleFunc("POST /api/v1/tags", router.tagsHandlers.PostTags)

	return router
}

// Helper function for sending a JSON response
func sendJSONResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
