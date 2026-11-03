package restapi

import (
	"errors"
	"net/http"
	"strings"

	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/devicetoken"
)

// The REST layer only reads the Authorization header: what a bearer names —
// the Device, and so the Project — is the Runtime node's device token
// service's (ADR 0004, amendment 2026-11-01). A bearer without the Device
// Token prefix is not a Device Token (a user's bearer is task 50): it is
// refused as unknown.

// bearerSecret returns the credentials of an Authorization header.
func bearerSecret(header string) (string, bool) {
	const scheme = "Bearer "
	if len(header) < len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	secret := strings.TrimSpace(header[len(scheme):])
	if secret == "" {
		return "", false
	}
	return secret, true
}

// authenticateDevice resolves the Authorization header a Device Gateway
// request must carry. On failure it has already sent the problem: 401 for a
// header that is missing, not a bearer, or a bearer that is no device token.
func authenticateDevice(w http.ResponseWriter, r *http.Request, tokens runtimeapp.DeviceTokenService) (runtimeapp.DeviceToken, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		sendProblem(w, NewUnauthorized("Authorization: Bearer <device token> is required", r.URL.Path))
		return runtimeapp.DeviceToken{}, false
	}
	if tokens == nil {
		sendProblem(w, NewUnauthorized("device tokens are not served here", r.URL.Path))
		return runtimeapp.DeviceToken{}, false
	}
	secret, ok := bearerSecret(header)
	if !ok {
		sendProblem(w, NewUnauthorized("Authorization must carry a device token: Bearer gsc_dev_…", r.URL.Path))
		return runtimeapp.DeviceToken{}, false
	}
	token, err := tokens.Authenticate(r.Context(), secret)
	switch {
	case err == nil:
		return token, true
	case errors.Is(err, devicetoken.ErrNotADeviceToken), errors.Is(err, devicetoken.ErrTokenNotFound):
		sendProblem(w, NewUnauthorized("the device token is unknown or revoked", r.URL.Path))
		return runtimeapp.DeviceToken{}, false
	default:
		sendInternalError(w, r, err)
		return runtimeapp.DeviceToken{}, false
	}
}

// deviceWriter resolves the writer of a Tag value: the Device a Device Token
// names, or no Device at all when the request carries no Authorization header
// (people write Tags too; user authentication is task 50). present reports
// whether a token was presented and resolved; ok is false only after a
// problem was sent.
func deviceWriter(w http.ResponseWriter, r *http.Request, tokens runtimeapp.DeviceTokenService) (token runtimeapp.DeviceToken, present bool, ok bool) {
	if r.Header.Get("Authorization") == "" {
		return runtimeapp.DeviceToken{}, false, true
	}
	token, ok = authenticateDevice(w, r, tokens)
	if !ok {
		return runtimeapp.DeviceToken{}, true, false
	}
	return token, true, true
}
