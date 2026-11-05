package draftchange

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
)

// TestEncodeDecode_RoundTripsEveryKind: a DraftChange's states survive the
// encoding, a kind and an ID each (ADR 0017).
func TestEncodeDecode_RoundTripsEveryKind(t *testing.T) {
	id := uuid.New()
	supplier := uuid.New()
	snapshots := []draft.Snapshot{
		{Kind: draft.KindTag, ID: id, State: draft.TagState{Name: "speed", Type: "integer", Supplier: &supplier}},
		{Kind: draft.KindScene, ID: id, State: draft.SceneState{Name: "s", Width: 10, Height: 20, BackgroundHTML: "<b/>"}},
		{Kind: draft.KindWidget, ID: id, State: draft.WidgetState{
			SceneID: id, Name: "w", X: 1, Y: 2, Z: 3, Width: 4, Height: 5,
			OriginX: 0.5, OriginY: 0.5, RotationDegrees: 45, TypeID: id,
			Labels: []string{"a"}, PortBindings: []draft.PortBindingState{{PortName: "value", TagID: id}},
		}},
		{Kind: draft.KindDevice, ID: id, State: draft.DeviceState{Name: "d", Protocol: "simulation", Config: json.RawMessage(`{"a":1}`)}},
		{Kind: draft.KindEnvironment, ID: id, State: draft.EnvironmentState{Name: "e"}},
		{Kind: draft.KindEnvironmentDevice, ID: id, State: draft.EnvironmentDeviceState{EnvironmentID: id, DeviceID: id, Params: json.RawMessage(`{}`)}},
		{Kind: draft.KindLibraryPin, ID: id, State: draft.LibraryPinState{LibraryID: id, Release: 2}},
	}
	encoded, err := Encode(snapshots)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded) != len(snapshots) {
		t.Fatalf("decoded %d snapshots, want %d", len(decoded), len(snapshots))
	}
	for i, want := range snapshots {
		got := decoded[i]
		if got.Kind != want.Kind || got.ID != want.ID {
			t.Errorf("snapshot %d: %s %s, want %s %s", i, got.Kind, got.ID, want.Kind, want.ID)
		}
		wantJSON, _ := json.Marshal(want.State)
		gotJSON, _ := json.Marshal(got.State)
		if string(wantJSON) != string(gotJSON) {
			t.Errorf("snapshot %d state: %s, want %s", i, gotJSON, wantJSON)
		}
	}
}

// TestDecode_UnknownFormat_IsRefused: a log written by a newer format is not
// guessed.
func TestDecode_UnknownFormat_IsRefused(t *testing.T) {
	for _, raw := range []string{
		`{"format": 99, "entities": []}`,
		`{"format": 1, "entities": [{"kind": "unknown", "id": "` + uuid.New().String() + `", "state": {}}]}`,
		`not json`,
	} {
		if _, err := Decode([]byte(raw)); err == nil {
			t.Errorf("Decode(%s): expected an error", raw)
		}
	}
}
