package restapi

import (
	"fmt"
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// Base problem type URIs
const (
	// Base URI for problem types
	BaseTypeURI = "https://api.growscada.com/errors/"

	// Standard problem types
	TypeBadRequest      = BaseTypeURI + "bad-request"
	TypeUnauthorized    = BaseTypeURI + "unauthorized"
	TypeForbidden       = BaseTypeURI + "forbidden"
	TypeNotFound        = BaseTypeURI + "not-found"
	TypeConflict        = BaseTypeURI + "conflict"
	TypeGone            = BaseTypeURI + "gone"
	TypeTooManyRequests = BaseTypeURI + "too-many-requests"
	TypeInternalError   = BaseTypeURI + "internal-error"
	TypeUnavailable     = BaseTypeURI + "service-unavailable"
)

// Common problem creators. The body of every error response is an
// apiv0.Problem.
func NewBadRequest(detail, instance string) *apiv0.Problem {
	return &apiv0.Problem{
		Type:     TypeBadRequest,
		Title:    "Bad Request",
		Status:   http.StatusBadRequest, //400
		Detail:   detail,
		Instance: instance,
	}
}

func NewNotFound(resourceType, resourceID, instance string) *apiv0.Problem {
	detail := fmt.Sprintf("%s with ID '%s' not found", resourceType, resourceID)
	return &apiv0.Problem{
		Type:     TypeNotFound,
		Title:    "Resource Not Found",
		Status:   http.StatusNotFound, //404
		Detail:   detail,
		Instance: instance,
	}
}

func NewConflict(resource, detail, instance string) *apiv0.Problem {
	return &apiv0.Problem{
		Type:     TypeConflict,
		Title:    fmt.Sprintf("Conflict: %s", resource),
		Status:   http.StatusConflict, //409
		Detail:   detail,
		Instance: instance,
	}
}

func NewGone(detail, instance string) *apiv0.Problem {
	return &apiv0.Problem{
		Type:     TypeGone,
		Title:    "Gone",
		Status:   http.StatusGone, // 410
		Detail:   detail,
		Instance: instance,
	}
}

func NewInternalError(detail, instance string) *apiv0.Problem {
	return &apiv0.Problem{
		Type:     TypeInternalError,
		Title:    "Internal Server Error",
		Status:   http.StatusInternalServerError, // 500
		Detail:   detail,
		Instance: instance,
	}
}

func NewServiceUnavailableError(detail, instance string) *apiv0.Problem {
	return &apiv0.Problem{
		Type:     TypeUnavailable,
		Title:    "Service Unavailable",
		Status:   http.StatusServiceUnavailable, // 503
		Detail:   detail,
		Instance: instance,
	}
}
