package restapi

import (
	"encoding/json"
	"net/http"

	"gitverse.ru/kipitix/growscada/internal/application"
	"gitverse.ru/kipitix/growscada/internal/application/dto"
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

// GetTags обрабатывает GET /tags запрос для получения списка тегов
func (h TagsHandlers) GetTags(w http.ResponseWriter, r *http.Request) {
	tagList, err := h.service.FindAllTags(r.Context())
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

// GetTagsByID обрабатывает GET /tags/{id}
func (h TagsHandlers) GetTagsByID(w http.ResponseWriter, r *http.Request) {
	// TODO: реализация
	w.WriteHeader(http.StatusNotAcceptable)
}

// PostTags обрабатывает POST /tags для создания нового тега
func (h TagsHandlers) PostTags(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateTagRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp := NewBadRequest(err.Error(), r.URL.Path)
		sendJSONResponse(w, http.StatusBadRequest, resp)
		return
	}

	// h.service.CreateTag(req)

	// Валидация (можно использовать валидатор, например go-playground/validator)
	// if err := validateCreateTag(req); err != nil {
	// 	sendJSONError(w, "Validation failed", http.StatusBadRequest, err.Error())
	// 	return
	// }

	// Создание тега через сервис
	// createdTag, err := h.tagService.CreateTag(r.Context(), req.Name, req.Kind, req.Value, req.Quality)
	// if err != nil {
	// 	sendJSONError(w, "Failed to create tag", http.StatusInternalServerError, err.Error())
	// 	return
	// }

	// // Формирование ответа
	// response := dto.TagResponse{
	// 	Tag: dto.NewTag(*createdTag),
	// }

	// sendJSONResponse(w, http.StatusCreated, response)

}
