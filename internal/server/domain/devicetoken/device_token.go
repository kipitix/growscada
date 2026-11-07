// Package devicetoken holds the Device Token: the secret by which a Device
// Gateway acts for one Device — it writes that Device's Tags and nothing else
// (ADR 0004, amendment 2026-11-01). A token is issued on the Runtime node,
// stored as its hash and never part of a Deployment; it outlives Deploys and
// Rollbacks and sleeps while its Device is not in the running Deployment. The
// aggregate itself lives in the Runtime node's schema; this package holds the
// secret's value object, the record the Runtime node stores and the
// repository interface.
package devicetoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

var (
	// ErrTokenNotFound is returned when a Device has no token, or the token a
	// request presents is unknown or revoked.
	ErrTokenNotFound = errors.New("device token not found")
	// ErrTokenExists is returned by Issue when the Device already has a token
	// that was not revoked: there is one token per Device.
	ErrTokenExists = errors.New("device token already issued")
	// ErrNotADeviceToken is returned for a bearer that does not carry the
	// Device Token prefix — a user's token (task 50), which the API does not
	// accept yet.
	ErrNotADeviceToken = errors.New("not a device token")
)

// Prefix distinguishes a Device Token from a future user bearer (task 50). A
// bearer without it is not a Device Token.
const Prefix = "gsc_dev_"

// secretBytes is the randomness of a token: 256 bits, which no one guesses.
const secretBytes = 32

// fingerprintHexChars is how much of the hash the fingerprint shows: enough to
// tell two tokens apart, far too little to guess the token.
const fingerprintHexChars = 8

// Secret is the plaintext of a Device Token: the value handed to the Gateway
// once. The Runtime node stores its Hash and its Fingerprint alone.
type Secret struct {
	raw string
}

// NewSecret generates a new random token.
func NewSecret() (Secret, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return Secret{}, fmt.Errorf("cannot generate a device token: %w", err)
	}
	return Secret{raw: Prefix + hex.EncodeToString(buf)}, nil
}

// ParseSecret reads a token a client presented. A value without the Device
// Token prefix is ErrNotADeviceToken, never a token to look up.
// Returns an error for anything that cannot be a token.
func ParseSecret(raw string) (Secret, error) {
	rest, ok := strings.CutPrefix(raw, Prefix)
	if !ok {
		return Secret{}, ErrNotADeviceToken
	}
	if len(rest) != secretBytes*2 {
		return Secret{}, fmt.Errorf("%w: malformed", ErrNotADeviceToken)
	}
	if _, err := hex.DecodeString(rest); err != nil {
		return Secret{}, fmt.Errorf("%w: malformed", ErrNotADeviceToken)
	}
	return Secret{raw: raw}, nil
}

// String is the token itself: the secret shown to the Gateway once.
func (s Secret) String() string { return s.raw }

// Hash is what the Runtime node stores and looks the token up by: the hex
// SHA-256 of the token.
func (s Secret) Hash() string {
	sum := sha256.Sum256([]byte(s.raw))
	return hex.EncodeToString(sum[:])
}

// Fingerprint is the token's public name, with no secret in it: the prefix
// and the first characters of the hash, by which people tell one token from
// another.
func (s Secret) Fingerprint() string {
	return FingerprintOfHash(s.Hash())
}

// FingerprintOfHash derives a stored token's fingerprint from its hash, so a
// token read back from the database shows the same name it was issued with.
func FingerprintOfHash(hash string) string {
	if len(hash) > fingerprintHexChars {
		hash = hash[:fingerprintHexChars]
	}
	return Prefix + hash
}

// Token is a Device's token as the Runtime node holds it, without the secret.
type Token struct {
	// DeviceID is the Device the token acts for; ProjectID is that Device's
	// project, which the token therefore names without a lookup elsewhere.
	DeviceID    id.ID[device.Device]
	ProjectID   id.ID[project.Project]
	Fingerprint string
	IssuedAt    time.Time
}

// NewToken is the record of a newly issued token for a Device of a project.
func NewToken(deviceID id.ID[device.Device], projectID id.ID[project.Project], secret Secret, issuedAt time.Time) Token {
	return Token{
		DeviceID:    deviceID,
		ProjectID:   projectID,
		Fingerprint: secret.Fingerprint(),
		IssuedAt:    issuedAt.UTC(),
	}
}

// Repository stores the tokens of the Devices in the Runtime node's schema.
// It never keeps the secret, only its hash.
type Repository interface {
	// Insert stores a new token of its Device.
	// Returns ErrTokenExists if the Device already has one.
	Insert(context.Context, Token, string) error
	// Replace stores the token of its Device, replacing the one it had: a
	// rotation. A Device without one gets it.
	Replace(context.Context, Token, string) error
	// FindByHash returns the token with the given hash, the lookup of
	// authentication.
	// Returns ErrTokenNotFound if there is none (revoked included).
	FindByHash(context.Context, string) (Token, error)
	// FindByDevice returns the token of the Device of the Project.
	// Returns ErrTokenNotFound if there is none.
	FindByDevice(context.Context, id.ID[device.Device]) (Token, error)
	// Delete removes the token of the Device of the Project.
	// Returns ErrTokenNotFound if there is none.
	Delete(context.Context, id.ID[device.Device], id.ID[project.Project]) error
	// DeleteByProject removes every token of the Project; a Project without
	// tokens is not an error. The row of the Project takes its tokens with it
	// (the foreign key cascades), so this only matters to a Runtime node of
	// its own (task 52); it makes the removal idempotent either way.
	DeleteByProject(context.Context, id.ID[project.Project]) error
}
