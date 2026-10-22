package uiutil

import (
	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// The UI keeps IDs as uuid.UUID, uuid.Nil meaning "none selected"; text is
// parsed only where it enters: LocalStorage, the event stream, DOM attributes
// and drag data.

// ParseID parses an ID read from the DOM, drag data or the event stream; text
// that is not a UUID (an empty <option>, foreign drag data) is uuid.Nil.
func ParseID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// LoadID reads the ID stored under key in LocalStorage; uuid.Nil if there is
// none or it does not parse.
func LoadID(ctx app.Context, key string) uuid.UUID {
	var s string
	if err := ctx.LocalStorage().Get(key, &s); err != nil {
		return uuid.Nil
	}
	return ParseID(s)
}

// StoreID writes id under key in LocalStorage; uuid.Nil is stored as "".
func StoreID(ctx app.Context, key string, id uuid.UUID) {
	s := ""
	if id != uuid.Nil {
		s = id.String()
	}
	ctx.LocalStorage().Set(key, s)
}
