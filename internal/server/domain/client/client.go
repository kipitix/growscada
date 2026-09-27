// Package client will host the Client aggregate (a connected SSE client and,
// eventually, its access rights). For now it exists only as an identity type
// for connection-lifecycle domain events; no aggregate behavior is defined
// yet.
package client

// Client - identity marker for a connected SSE client.
type Client interface {
}
