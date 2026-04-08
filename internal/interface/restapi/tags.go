package restapi

import (
	"encoding/json"
	"net/http"

	"gitverse.ru/kipitix/growscada/internal/application"
)

// TagsHandlers отвечает за обработку HTTP запросов, связанных с тегами.
// Он зависит от абстракции прикладного слоя.
type TagsHandlers struct {
	service application.TagService
}

// NewTagsHandler создает новый обработчик для тегов
// Принимает сервис тегов и возвращает указатель на TagsHandlers
func NewTagsHandler(s application.TagService) *TagsHandlers {
	return &TagsHandlers{service: s}
}

// GetTags обрабатывает GET запрос для получения списка тегов
// В текущей реализации просто возвращает статус 200 OK, логика находится в разработке
func (h TagsHandlers) GetTags(w http.ResponseWriter, r *http.Request) {
	tagList, err := h.service.TagList(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonData, err := json.Marshal(tagList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(jsonData); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Закомментированные функции для примера будущей реализации
// GetTag обрабатывает GET /tags/{name}
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
