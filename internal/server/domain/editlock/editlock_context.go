package editlock

import "context"

// Identity is what a write knows about the client on whose behalf it runs:
// the key of a request, a request that named no client, or the server itself.
// It travels in the context, never in the aggregate being written (ADR 0015).
type Identity int

const (
	// IdentityNone is a context that never passed a request boundary: no
	// identity was set. A gated write refuses it as ErrNoClient, so a
	// forgotten key fails closed.
	IdentityNone Identity = iota
	// IdentityClient is a request that named its client: the key it carries
	// is the only one allowed to write the Draft it holds.
	IdentityClient
	// IdentityNoClient is a request that named no client: any gated write
	// it makes is ErrNoClient.
	IdentityNoClient
	// IdentitySystem is the server itself — a policy or a cleanup: it writes
	// the Draft without holding the lock.
	IdentitySystem
)

type identityKey struct{}
type clientKey struct{}

// WithClient marks ctx as a request of the client that holds the key.
func WithClient(ctx context.Context, key Key) context.Context {
	ctx = context.WithValue(ctx, clientKey{}, key)
	return context.WithValue(ctx, identityKey{}, IdentityClient)
}

// WithoutClient marks ctx as a request that named no client.
func WithoutClient(ctx context.Context) context.Context {
	return context.WithValue(ctx, identityKey{}, IdentityNoClient)
}

// System marks ctx as the server's own write: a policy or a cleanup, which
// passes no EditLock check.
func System(ctx context.Context) context.Context {
	return context.WithValue(ctx, identityKey{}, IdentitySystem)
}

// IdentityFrom returns the identity of ctx; IdentityNone when none was set.
func IdentityFrom(ctx context.Context) Identity {
	if id, ok := ctx.Value(identityKey{}).(Identity); ok {
		return id
	}
	return IdentityNone
}

// ClientFrom returns the key of the client a request named, if it named one.
func ClientFrom(ctx context.Context) (Key, bool) {
	key, ok := ctx.Value(clientKey{}).(Key)
	if !ok || key.IsZero() {
		return Key{}, false
	}
	return key, true
}
