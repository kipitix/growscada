package restapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kipitix/growscada/internal/application"
	"github.com/kipitix/growscada/internal/domain/indicator_type"
	"github.com/kipitix/growscada/internal/interface/restapi/rest_dto"
)

// IndicatorTypesHandlers handles HTTP requests related to indicator types.
type IndicatorTypesHandlers struct {
	service application.IndicatorTypeService
}

func NewIndicatorTypesHandler(s application.IndicatorTypeService) *IndicatorTypesHandlers {
	return &IndicatorTypesHandlers{service: s}
}

// GetIndicatorTypes handles GET /indicator-types
func (h IndicatorTypesHandlers) GetIndicatorTypes(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.FindAllIndicatorTypes(r.Context())
	if err != nil {
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}
	sendJSONResponse(w, http.StatusOK, rest_dto.NewGetIndicatorTypesResponse(list))
}

// GetIndicatorTypesByID handles GET /indicator-types/{id}
func (h IndicatorTypesHandlers) GetIndicatorTypesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := indicator_type.ParseIndicatorTypeID(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	found, err := h.service.FindIndicatorTypeByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, indicator_type.ErrIndicatorTypeNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusOK, rest_dto.NewIndicatorTypeResponse(found))
}

// PostIndicatorTypes handles POST /indicator-types
func (h IndicatorTypesHandlers) PostIndicatorTypes(w http.ResponseWriter, r *http.Request) {
	var request rest_dto.CreateIndicatorTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	created, err := h.service.CreateIndicatorType(r.Context(), rest_dto.NewCreateIndicatorTypeInput(request))
	if err != nil {
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusCreated, rest_dto.NewCreateIndicatorTypeResponse(created))
}

// PutIndicatorTypesByID handles PUT /indicator-types/{id}
func (h IndicatorTypesHandlers) PutIndicatorTypesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := indicator_type.ParseIndicatorTypeID(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	var request rest_dto.UpdateIndicatorTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	updated, err := h.service.UpdateIndicatorType(r.Context(), rest_dto.NewUpdateIndicatorTypeInput(request, id.UUID()))
	if err != nil {
		if errors.Is(err, indicator_type.ErrIndicatorTypeNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusOK, rest_dto.NewUpdateIndicatorTypeResponse(updated))
}

// DeleteIndicatorTypesByID handles DELETE /indicator-types/{id}
func (h IndicatorTypesHandlers) DeleteIndicatorTypesByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := indicator_type.ParseIndicatorTypeID(idStr)
	if err != nil {
		sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(err.Error(), r.URL.Path))
		return
	}

	deleted, err := h.service.DeleteIndicatorTypeByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, indicator_type.ErrIndicatorTypeNotFound) {
			sendJSONResponse(w, http.StatusNotFound, NewNotFound(err.Error(), idStr, r.URL.Path))
			return
		}
		sendJSONResponse(w, http.StatusInternalServerError, NewInternalError(err.Error(), r.URL.Path))
		return
	}

	sendJSONResponse(w, http.StatusOK, rest_dto.NewIndicatorTypeResponse(deleted))
}
