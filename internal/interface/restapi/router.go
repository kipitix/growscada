package restapi

import (
	"encoding/json"
	"net/http"

	"github.com/kipitix/growscada/internal/application"
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
	router.serveMux.HandleFunc("GET /api/v1/tags/{id}", router.tagsHandlers.GetTagsByID)
	router.serveMux.HandleFunc("POST /api/v1/tags", router.tagsHandlers.PostTags)
	router.serveMux.HandleFunc("DELETE /api/v1/tags/{id}", router.tagsHandlers.DeleteTagByID)
	router.serveMux.HandleFunc("PATCH /api/v1/tags/{id}/value", router.tagsHandlers.PatchTagValue)

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
