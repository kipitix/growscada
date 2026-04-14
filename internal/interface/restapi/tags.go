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
	var request dto.CreateTagRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errorResponse := NewBadRequest(err.Error(), r.URL.Path)
		sendJSONResponse(w, http.StatusBadRequest, errorResponse)
		return
	}

	response, err := h.service.CreateTag(r.Context(), request)
	if err != nil {
		errorResponse := NewInternalError(err.Error(), r.URL.Path)
		sendJSONResponse(w, http.StatusInternalServerError, errorResponse)
	}

	sendJSONResponse(w, http.StatusCreated, response)
}
