// Package devicelink is the contract by which a Device delivers Tag values to
// the GrowSCADA server (ADR 0004). The device simulator is its first user;
// future protocol adapters (Modbus, OPC-UA, MQTT) deliver values the same way.
package devicelink

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/apiclient"
)

// ErrTagNotFound means the Device's project has no tag with the given name,
// or the tag (or the project) was deleted while the Device was writing it.
var ErrTagNotFound = errors.New("tag not found")

// Tag is a server tag a Device writes: its identity and TagType.
type Tag struct {
	ID   uuid.UUID
	Name string
	Type TagType
}

// Link delivers Tag values to the server, to the Tags of the Device's
// project: the project a Device Token names (ADR 0004, amendment 2026-11-03).
type Link interface {
	// Resolve looks a tag up by its name, unique in the project (ADR 0003).
	// The error matches ErrTagNotFound if there is none.
	Resolve(ctx context.Context, name string) (Tag, error)
	// Write sets a tag's value and quality. The Device is the source of truth:
	// the Link resolves optimistic-locking conflicts itself. The error matches
	// ErrTagNotFound if the tag is gone.
	Write(ctx context.Context, tag Tag, value string, quality Quality) error
}

// IsTransient reports whether an error is worth retrying later: the server is
// unreachable or failed (5xx), as opposed to rejecting the request.
func IsTransient(err error) bool {
	// *url.Error is a net.Error whatever it wraps: judge by the cause, so a
	// bad URL scheme or a TLS certificate error is not retried.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true // the server closed the connection, e.g. while restarting
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var statusErr *apiclient.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Problem.Status >= http.StatusInternalServerError
	}
	return false
}

// IsNoPermission reports whether the server refused the Device: its token is
// unknown or revoked (401), or the Device may not do what it asked (403).
func IsNoPermission(err error) bool {
	return IsUnauthenticated(err) || IsForbidden(err)
}

// IsUnauthenticated reports whether the server refused the Device's token:
// it is unknown or revoked (401). Nothing the Gateway retries brings it back,
// so a Gateway ends.
func IsUnauthenticated(err error) bool {
	return hasStatus(err, http.StatusUnauthorized)
}

// IsForbidden reports whether the server refused the Device's request: it may
// not do what it asked (403) — it is not the Tag's supplier, or it is not in
// the configuration it asked about. A Gateway stops writing what a write
// refusal names, and waits for a configuration refusal to pass.
func IsForbidden(err error) bool {
	return hasStatus(err, http.StatusForbidden)
}

func hasStatus(err error, status int) bool {
	var statusErr *apiclient.StatusError
	return errors.As(err, &statusErr) && statusErr.Problem.Status == status
}
