package restapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ProblemDetails реализует RFC 9457
// https://www.rfc-editor.org/rfc/rfc9457.html
type ProblemDetails struct {
	// Type - ссылка на URI, идентифицирующая тип проблемы
	// ОБЯЗАТЕЛЬНО. Должна быть доступной для чтения человеком документацией
	Type string `json:"type"`

	// Title - краткое, понятное описание типа проблемы
	// ОБЯЗАТЕЛЬНО. Не должно меняться между случаями возникновения
	Title string `json:"title"`

	// Status - код состояния HTTP, сгенерированный сервером-источником
	// ОБЯЗАТЕЛЬНО. ДОЛЖЕН соответствовать коду состояния ответа HTTP
	Status int `json:"status"`

	// Detail - понятное человеку объяснение, специфичное для данного случая
	// НЕОБЯЗАТЕЛЬНО. Помогает клиенту понять ошибку
	Detail string `json:"detail,omitempty"`

	// Instance - ссылка на URI, идентифицирующая конкретный случай
	// НЕОБЯЗАТЕЛЬНО. Может использоваться для поиска проблемы в логах
	Instance string `json:"instance,omitempty"`

	// Extensions - дополнительные сведения о проблеме (RFC 9457 разрешает пользовательские поля)
	// НЕ ДОЛЖНЫ переопределять стандартные поля выше
	Extensions map[string]interface{} `json:"-"`
}

// MarshalJSON реализует пользовательскую маршалиацию для расширений
func (p ProblemDetails) MarshalJSON() ([]byte, error) {
	// Создаем карту для базовых полей
	result := map[string]interface{}{
		"type":   p.Type,
		"title":  p.Title,
		"status": p.Status,
	}

	// Добавляем необязательные поля, если они присутствуют
	if p.Detail != "" {
		result["detail"] = p.Detail
	}
	if p.Instance != "" {
		result["instance"] = p.Instance
	}

	// Объединяем расширения
	for k, v := range p.Extensions {
		// Предотвращаем переопределение стандартных полей
		if _, exists := result[k]; !exists {
			result[k] = v
		}
	}

	return json.Marshal(result)
}

// UnmarshalJSON реализует пользовательскую анмаршалиацию для расширений
func (p *ProblemDetails) UnmarshalJSON(data []byte) error {
	// Сначала анмаршалим в карту, чтобы захватить расширения
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Извлекаем стандартные поля
	if v, ok := raw["type"].(string); ok {
		p.Type = v
		delete(raw, "type")
	}
	if v, ok := raw["title"].(string); ok {
		p.Title = v
		delete(raw, "title")
	}
	if v, ok := raw["status"].(float64); ok {
		p.Status = int(v)
		delete(raw, "status")
	}
	if v, ok := raw["detail"].(string); ok {
		p.Detail = v
		delete(raw, "detail")
	}
	if v, ok := raw["instance"].(string); ok {
		p.Instance = v
		delete(raw, "instance")
	}

	// Всё остальное — расширения
	if len(raw) > 0 {
		p.Extensions = raw
	}

	return nil
}

// Базовые URI типов проблем
const (
	// Базовый URI для типов проблем
	BaseTypeURI = "https://api.growscada.com/errors/"

	// Стандартные типы проблем
	TypeBadRequest      = BaseTypeURI + "bad-request"
	TypeUnauthorized    = BaseTypeURI + "unauthorized"
	TypeForbidden       = BaseTypeURI + "forbidden"
	TypeNotFound        = BaseTypeURI + "not-found"
	TypeConflict        = BaseTypeURI + "conflict"
	TypeValidation      = BaseTypeURI + "validation-error"
	TypeTooManyRequests = BaseTypeURI + "too-many-requests"
	TypeInternalError   = BaseTypeURI + "internal-error"
	TypeUnavailable     = BaseTypeURI + "service-unavailable"
)

// Common problem creators
func NewBadRequest(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeBadRequest,
		Title:    "Bad Request",
		Status:   http.StatusBadRequest, //400
		Detail:   detail,
		Instance: instance,
	}
}

func NewNotFound(resourceType, resourceID, instance string) *ProblemDetails {
	detail := fmt.Sprintf("%s with ID '%s' not found", resourceType, resourceID)
	return &ProblemDetails{
		Type:     TypeNotFound,
		Title:    "Resource Not Found",
		Status:   http.StatusNotFound, //404
		Detail:   detail,
		Instance: instance,
	}
}

func NewConflict(resource, detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeConflict,
		Title:    fmt.Sprintf("Conflict: %s", resource),
		Status:   http.StatusConflict, //409
		Detail:   detail,
		Instance: instance,
	}
}

func NewValidationError(validationErrors map[string][]string, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeValidation,
		Title:    "Validation Error",
		Status:   http.StatusUnprocessableEntity, // 422
		Detail:   "Request validation failed",
		Instance: instance,
		Extensions: map[string]interface{}{
			"errors": validationErrors,
		},
	}
}

func NewInternalError(detail, instance string) *ProblemDetails {
	return &ProblemDetails{
		Type:     TypeInternalError,
		Title:    "Internal Server Error",
		Status:   http.StatusInternalServerError, // 500
		Detail:   detail,
		Instance: instance,
	}
}
