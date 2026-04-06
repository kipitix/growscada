package restapi

import (
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
	// Создаем роутер - контейнер для обработчиков
	router := &APIRouter{}

	// Создаем mux и регистрируем обработчики
	router.serveMux = http.NewServeMux()

	// Создаем обработчики - обертки над сервисами
	router.tagsHandlers = NewTagsHandler(tagService)

	// Регистрируем обработчики
	router.serveMux.HandleFunc("GET /api/v1/tags", router.tagsHandlers.GetTags)

	return router
}
