package uiutil

import (
	"slices"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// IdxByID returns the index of the item id in items; -1 if absent or uuid.Nil.
func IdxByID[T any](items []T, id uuid.UUID, idOf func(T) uuid.UUID) int {
	if id == uuid.Nil {
		return -1
	}
	return slices.IndexFunc(items, func(it T) bool { return idOf(it) == id })
}

// FindByID returns the item id in items, or false if there is none.
func FindByID[T any](items []T, id uuid.UUID, idOf func(T) uuid.UUID) (T, bool) {
	if i := IdxByID(items, id, idOf); i >= 0 {
		return items[i], true
	}
	var zero T
	return zero, false
}

// The idOf functions of IdxByID and FindByID for the API's types.

func ProjectID(p apiv0.ProjectResponse) uuid.UUID        { return p.ID }
func SceneID(sc apiv0.SceneResponse) uuid.UUID           { return sc.ID }
func WidgetID(w apiv0.WidgetResponse) uuid.UUID          { return w.ID }
func WidgetTypeID(wt apiv0.WidgetTypeResponse) uuid.UUID { return wt.ID }
func TagID(t apiv0.TagResponse) uuid.UUID                { return t.ID }
