package apiv0

import (
	"github.com/google/uuid"
)

// DeviceIdentityResponse is what a Device Token acts for: the Device and the
// Project that owns it. A Gateway reads it before anything else, because every
// write names its Project in the path and the token is the only thing that
// names the Project.
type DeviceIdentityResponse struct {
	ProjectID uuid.UUID `json:"project_id"`
	DeviceID  uuid.UUID `json:"device_id"`
}
