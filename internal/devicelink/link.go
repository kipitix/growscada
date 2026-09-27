// Package devicelink is the contract by which a Device delivers Tag values to
// the GrowSCADA server (ADR 0004). The device simulator is its first user;
// future protocol adapters (Modbus, OPC-UA, MQTT) deliver values the same way.
package devicelink

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/apiclient"
)

// ErrTagNotFound means the server has no tag with the given name, or the tag
// was deleted while the Device was writing it.
var ErrTagNotFound = errors.New("tag not found")

// Tag is a server tag a Device writes: its identity and TagType.
type Tag struct {
	ID   uuid.UUID
	Name string
	Type string // "string", "boolean" or "integer"
}

// Link delivers Tag values to the server.
type Link interface {
	// Resolve looks a tag up by its unique name (ADR 0003). The error matches
	// ErrTagNotFound if there is none.
	Resolve(ctx context.Context, name string) (Tag, error)
	// Write sets a tag's value and quality. The Device is the source of truth:
	// the Link resolves optimistic-locking conflicts itself. The error matches
	// ErrTagNotFound if the tag is gone.
	Write(ctx context.Context, tag Tag, value, quality string) error
}

// IsTransient reports whether an error is worth retrying later: the server is
// unreachable or failed (5xx), as opposed to rejecting the request.
func IsTransient(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var statusErr *apiclient.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status >= http.StatusInternalServerError
	}
	return false
}
