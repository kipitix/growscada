package apiv0

import (
	"time"

	"github.com/google/uuid"
)

// DeviceTokenResponse is the HTTP DTO that answers issuing or rotating a
// Device Token: the token itself. It is the only time the secret is ever
// returned — the server stores its hash and the fingerprint alone.
type DeviceTokenResponse struct {
	Token string `json:"token"`
}

// DeviceTokenMetadataResponse is the HTTP DTO of the token of a Device
// without its secret: the Device it acts for, its fingerprint (the short
// identifier by which people tell one token from another) and when it was
// issued.
type DeviceTokenMetadataResponse struct {
	DeviceID    uuid.UUID `json:"device_id"`
	Fingerprint string    `json:"fingerprint"`
	IssuedAt    time.Time `json:"issued_at"`
}
