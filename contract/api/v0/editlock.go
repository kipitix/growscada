package apiv0

import "github.com/google/uuid"

// ClientHeader is the header every gated request of a Draft carries: the key
// the client (a browser tab) keeps for itself (ADR 0015). It is coordination,
// not authentication: the key is no secret and names no User.
const ClientHeader = "X-GrowSCADA-Client"

// EventsQueryClient is the parameter of an event stream naming the client
// that follows it, as ClientHeader names it on a request. While the stream is
// open the client counts as alive: its EditLock survives without requests. It
// is optional — a Device Gateway, the History mode and any other reader that
// edits nothing follow the stream without one.
const EventsQueryClient = "client"

// EditLockPath is the path, under a project's path or a library's, of its
// Draft's EditLock: GET reads it, POST takes it (Check out), DELETE gives it
// back (Check in), and /takeover takes it from its holder.
const EditLockPath = "/edit-lock"

// EditLockResponse is the state of a Draft's EditLock. A free lock has no
// Holder; its Version keeps growing across holders, so a client compares the
// one it saw with the current one. Version is 0 for a Draft whose lock was
// never taken.
type EditLockResponse struct {
	Held    bool                    `json:"held"`
	Holder  *EditLockHolderResponse `json:"holder,omitempty"`
	Version int                     `json:"version"`
}

// EditLockHolderResponse is who holds a Draft's EditLock: the client key (so
// the holder recognizes itself), the name it declared, and since when.
type EditLockHolderResponse struct {
	Key   uuid.UUID `json:"key"`
	Name  string    `json:"name"`
	Since string    `json:"since"`
}

// CheckOutEditLockRequest is the body of a Check out: the name the holder
// declares. The client key is the request's header.
type CheckOutEditLockRequest struct {
	Name string `json:"name"`
}

// TakeOverEditLockRequest is the body of a takeover: the name the new holder
// declares and the Version of the lock it observed. A stale Version is 409.
type TakeOverEditLockRequest struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}
