package restapi

import (
	"net/http"

	"gitverse.ru/kipitix/growscada/internal/application"
)

// import (
// 	"encoding/json"
// 	"net/http"
// )

// TagsHandlers отвечает за обработку HTTP запросов, связанных с тегами.
// Он зависит от абстракции прикладного слоя.
type TagsHandlers struct {
	service application.TagService
}

func NewTagsHandler(s application.TagService) *TagsHandlers {
	return &TagsHandlers{service: s}
}

func (h *TagsHandlers) GetTags(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// // GetTag обрабатывает GET /tags/{name}
// func (h *TagHandler) GetTag(w http.ResponseWriter, r *http.Request) {
// 	// В реальном проекте здесь нужно вытащить name из path params
// 	// Для примера берем из query ?name=...
// 	name := r.URL.Query().Get("name")
// 	if name == "" {
// 		http.Error(w, "name parameter is required", http.StatusBadRequest)
// 		return
// 	}

// 	t, err := h.service.GetTag(r.Context(), name)
// 	if err != nil {
// 		// Простая обработка ошибок, в продакшене нужен логгер
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.Header().Set("Content-Type", "application/json")
// 	json.NewEncoder(w).Encode(t)
// }

// // WriteTag обрабатывает POST /tags/write
// func (h *TagHandler) WriteTag(w http.ResponseWriter, r *http.Request) {
// 	var req struct {
// 		Name  string  `json:"name"`
// 		Value float64 `json:"value"`
// 	}

// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		http.Error(w, "invalid json", http.StatusBadRequest)
// 		return
// 	}

// 	if err := h.service.WriteTag(r.Context(), req.Name, req.Value); err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	w.WriteHeader(http.StatusOK)
// }
