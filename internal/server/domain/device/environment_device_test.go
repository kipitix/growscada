package device_test

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

func TestCreateEnvironmentDevice_FieldsAndEvent(t *testing.T) {
	environmentID := id.NewID[device.Environment]()
	deviceID := id.NewID[device.Device]()

	pair := device.CreateEnvironmentDevice(testProject, environmentID, deviceID, params(t, `{"address":"10.0.0.1"}`))

	if pair.ProjectID() != testProject || pair.EnvironmentID() != environmentID ||
		pair.DeviceID() != deviceID || pair.Params().String() != `{"address":"10.0.0.1"}` {
		t.Fatalf("unexpected pair %s", pair)
	}

	events := pair.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("PendingEvents = %v, want [environment_device_created]", events)
	}
	created, ok := events[0].(device.EnvironmentDeviceEvent)
	if !ok || created.Type() != event.EventTypeEnvironmentDeviceCreated ||
		created.EnvironmentID() != environmentID ||
		created.DeviceID() != deviceID || created.ProjectID() != testProject {
		t.Errorf("unexpected event %v", events[0])
	}
	if _, ok := any(events[0]).(project.ContentEvent); !ok {
		t.Error("an EnvironmentDevice event is content of its Project")
	}
}

func TestEnvironmentDevice_UpdateParams(t *testing.T) {
	environmentID := id.NewID[device.Environment]()
	deviceID := id.NewID[device.Device]()
	stored := device.ReconstituteEnvironmentDevice(testProject,
		environmentID, deviceID, params(t, `{"port":1}`))

	updated, err := stored.UpdateParams(params(t, `{"port":2}`))
	if err != nil {
		t.Fatalf("UpdateParams: %v", err)
	}
	if updated.Params().String() != `{"port":2}` || updated.EnvironmentID() != environmentID || updated.DeviceID() != deviceID {
		t.Errorf("unexpected pair %s", updated)
	}
	if stored.Params().String() != `{"port":1}` {
		t.Errorf("UpdateParams changed the pair it was called on: %s", stored)
	}
	events := updated.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("PendingEvents = %v, want [environment_device_updated]", events)
	}
	changed, ok := events[0].(device.EnvironmentDeviceEvent)
	if !ok || changed.Type() != event.EventTypeEnvironmentDeviceUpdated ||
		changed.EnvironmentID() != environmentID ||
		changed.DeviceID() != deviceID || changed.ProjectID() != testProject {
		t.Errorf("unexpected event %v", events[0])
	}
}

func TestEnvironmentDevice_Delete(t *testing.T) {
	environmentID := id.NewID[device.Environment]()
	deviceID := id.NewID[device.Device]()
	stored := device.ReconstituteEnvironmentDevice(testProject,
		environmentID, deviceID, device.EmptyEnvironmentDeviceParams())

	events := stored.Delete().PendingEvents()
	if len(events) != 1 {
		t.Fatalf("PendingEvents = %v, want [environment_device_deleted]", events)
	}
	deleted, ok := events[0].(device.EnvironmentDeviceEvent)
	if !ok || deleted.Type() != event.EventTypeEnvironmentDeviceDeleted || deleted.EnvironmentID() != environmentID ||
		deleted.DeviceID() != deviceID {
		t.Errorf("unexpected event %v", events[0])
	}
	if got := stored.PendingEvents(); len(got) != 0 {
		t.Errorf("Delete changed the pair it was called on: %v", got)
	}
}

func TestEnvironmentDeviceParams_IsCopiedInAndOut(t *testing.T) {
	raw := []byte(`{"port":1}`)
	params, err := device.NewEnvironmentDeviceParams(raw)
	if err != nil {
		t.Fatalf("NewEnvironmentDeviceParams: %v", err)
	}
	raw[2] = 'x'

	out := params.Bytes()
	out[2] = 'y'

	if got := params.String(); got != `{"port":1}` {
		t.Errorf("params = %s: neither the input nor the output is shared", got)
	}
}

func TestEnvironmentDeviceParams_ZeroValueIsTheEmptyObject(t *testing.T) {
	var zero device.EnvironmentDeviceParams
	if got := zero.String(); got != `{}` {
		t.Errorf("the zero params are %s, want the empty object", got)
	}
	if got := string(zero.Bytes()); got != `{}` {
		t.Errorf("the zero params' bytes are %s, want the empty object", got)
	}
}

func TestEnvironmentDeviceParams_RequiresAJSONObject(t *testing.T) {
	for _, raw := range []string{`[1]`, `"text"`, `{`, `null`} {
		if _, err := device.NewEnvironmentDeviceParams([]byte(raw)); err == nil {
			t.Errorf("NewEnvironmentDeviceParams(%q): expected an error", raw)
		}
	}
	empty, err := device.NewEnvironmentDeviceParams(nil)
	if err != nil || empty.String() != `{}` {
		t.Errorf("NewEnvironmentDeviceParams(nil) = %s, %v; want the empty object", empty, err)
	}
}
