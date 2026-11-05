// Package draftchange is the internal encoding of a DraftChange's before and
// after states (ADR 0017). It is not a contract: like the outbox's payload,
// it carries a format version and is read back only by the server, so a
// change of the shape is a change here, with the previous format kept until
// the stack drains.
package draftchange

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
)

// FormatVersion is the version of the encoding this package writes and the
// one it understands when reading.
const FormatVersion = 1

type document struct {
	Format   int      `json:"format"`
	Entities []entity `json:"entities"`
}

type entity struct {
	Kind  draft.Kind      `json:"kind"`
	ID    uuid.UUID       `json:"id"`
	State json.RawMessage `json:"state"`
}

// Encode encodes the snapshots of one side of a DraftChange. The empty list
// is the empty document.
func Encode(snapshots []draft.Snapshot) ([]byte, error) {
	doc := document{Format: FormatVersion, Entities: make([]entity, 0, len(snapshots))}
	for _, snapshot := range snapshots {
		state, err := json.Marshal(snapshot.State)
		if err != nil {
			return nil, fmt.Errorf("cannot encode the state of %s %s: %w", snapshot.Kind, snapshot.ID, err)
		}
		doc.Entities = append(doc.Entities, entity{Kind: snapshot.Kind, ID: snapshot.ID, State: state})
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("cannot encode draft change: %w", err)
	}
	return encoded, nil
}

// Decode decodes one side of a DraftChange as Encode wrote it. A format this
// package does not know is an error: the stack must be read, not guessed.
func Decode(data []byte) ([]draft.Snapshot, error) {
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("cannot decode draft change: %w", err)
	}
	if doc.Format != FormatVersion {
		return nil, fmt.Errorf("draft change format %d is not the supported %d", doc.Format, FormatVersion)
	}
	snapshots := make([]draft.Snapshot, 0, len(doc.Entities))
	for _, e := range doc.Entities {
		state, err := decodeState(e.Kind, e.State)
		if err != nil {
			return nil, fmt.Errorf("cannot decode the state of %s %s: %w", e.Kind, e.ID, err)
		}
		snapshots = append(snapshots, draft.Snapshot{Kind: e.Kind, ID: e.ID, State: state})
	}
	return snapshots, nil
}

func decodeState(kind draft.Kind, data []byte) (draft.State, error) {
	var state draft.State
	switch kind {
	case draft.KindTag:
		state = &draft.TagState{}
	case draft.KindScene:
		state = &draft.SceneState{}
	case draft.KindWidget:
		state = &draft.WidgetState{}
	case draft.KindDevice:
		state = &draft.DeviceState{}
	case draft.KindEnvironment:
		state = &draft.EnvironmentState{}
	case draft.KindEnvironmentDevice:
		state = &draft.EnvironmentDeviceState{}
	case draft.KindLibraryPin:
		state = &draft.LibraryPinState{}
	case draft.KindWidgetType:
		state = &draft.WidgetTypeState{}
	default:
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, err
	}
	// The pointer is only the unmarshal target: a Snapshot carries the value.
	switch typed := state.(type) {
	case *draft.TagState:
		return *typed, nil
	case *draft.SceneState:
		return *typed, nil
	case *draft.WidgetState:
		return *typed, nil
	case *draft.DeviceState:
		return *typed, nil
	case *draft.EnvironmentState:
		return *typed, nil
	case *draft.EnvironmentDeviceState:
		return *typed, nil
	case *draft.LibraryPinState:
		return *typed, nil
	case *draft.WidgetTypeState:
		return *typed, nil
	}
	return nil, fmt.Errorf("unknown state for kind %q", kind)
}
