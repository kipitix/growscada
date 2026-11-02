package device_test

import (
	"errors"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// testProject is the Project of the aggregates the tests build.
var testProject = id.NewID[project.Project]()

func deviceName(t *testing.T, s string) device.DeviceName {
	t.Helper()
	name, err := device.NewDeviceName(s)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func environmentName(t *testing.T, s string) device.EnvironmentName {
	t.Helper()
	name, err := device.NewEnvironmentName(s)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func config(t *testing.T, raw string) device.DeviceConfig {
	t.Helper()
	cfg, err := device.NewDeviceConfig([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func params(t *testing.T, raw string) device.EnvironmentDeviceParams {
	t.Helper()
	p, err := device.NewEnvironmentDeviceParams([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCreateDevice_FieldsAndEvent(t *testing.T) {
	anID := id.NewID[device.Device]()
	d, err := device.CreateDevice(anID, testProject, deviceName(t, "sim-1"), "the simulator",
		device.ProtocolSimulation, config(t, `{"patterns":{"pressure":{"min":1,"max":2}}}`))
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}

	if d.ID() != anID || d.ProjectID() != testProject || d.Name().String() != "sim-1" ||
		d.Comment() != "the simulator" || d.Protocol() != device.ProtocolSimulation ||
		d.Config().String() != `{"patterns":{"pressure":{"min":1,"max":2}}}` {
		t.Fatalf("unexpected device %s", d)
	}
	if d.Version() != version.Initial[device.Device]() {
		t.Errorf("Version = %s, want the initial one", d.Version())
	}

	events := d.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("PendingEvents = %v, want [device_created]", events)
	}
	created, ok := events[0].(device.DeviceCreatedEvent)
	if !ok || created.Type() != event.EventTypeDeviceCreated || created.DeviceID() != anID || created.ProjectID() != testProject {
		t.Errorf("unexpected event %v", events[0])
	}
	if _, ok := any(events[0]).(project.ContentEvent); !ok {
		t.Error("a Device event is content of its Project")
	}
}

func TestReconstituteDevice_RecordsNothing(t *testing.T) {
	d, err := device.ReconstituteDevice(id.NewID[device.Device](), testProject, deviceName(t, "sim-1"), "",
		device.ProtocolSimulation, device.EmptyDeviceConfig(), version.Committed[device.Device]())
	if err != nil {
		t.Fatalf("ReconstituteDevice: %v", err)
	}
	if got := d.PendingEvents(); len(got) != 0 {
		t.Errorf("PendingEvents = %v, want none", got)
	}
}

func TestDevice_Update(t *testing.T) {
	stored, err := device.ReconstituteDevice(id.NewID[device.Device](), testProject, deviceName(t, "sim-1"), "old",
		device.ProtocolSimulation, config(t, `{"a":1}`), version.Committed[device.Device]())
	if err != nil {
		t.Fatalf("ReconstituteDevice: %v", err)
	}

	if _, err := stored.Update(version.Initial[device.Device](), deviceName(t, "sim-2"), "", device.ProtocolSimulation, device.EmptyDeviceConfig()); !errors.Is(err, device.ErrDeviceConflict) {
		t.Errorf("stale update: %v, want ErrDeviceConflict", err)
	}

	updated, err := stored.Update(stored.Version(), deviceName(t, "sim-2"), "new", device.ProtocolSimulation, config(t, `{"b":2}`))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name().String() != "sim-2" || updated.Comment() != "new" || updated.Config().String() != `{"b":2}` {
		t.Errorf("unexpected updated device %s", updated)
	}
	if stored.Name().String() != "sim-1" || stored.Comment() != "old" || stored.Config().String() != `{"a":1}` {
		t.Errorf("Update changed the device it was called on: %s", stored)
	}
	if got := updated.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeDeviceUpdated {
		t.Errorf("PendingEvents = %v, want [device_updated]", got)
	}
	if got := stored.PendingEvents(); len(got) != 0 {
		t.Errorf("the stored device recorded %v", got)
	}
	if err := stored.CheckVersion(stored.Version()); err != nil {
		t.Errorf("CheckVersion of the current version: %v", err)
	}
}

func TestDevice_Delete(t *testing.T) {
	stored, err := device.ReconstituteDevice(id.NewID[device.Device](), testProject, deviceName(t, "sim-1"), "",
		device.ProtocolSimulation, device.EmptyDeviceConfig(), version.Committed[device.Device]())
	if err != nil {
		t.Fatalf("ReconstituteDevice: %v", err)
	}

	deleted := stored.Delete()
	if got := deleted.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeDeviceDeleted {
		t.Errorf("PendingEvents = %v, want [device_deleted]", got)
	}
	if got := stored.PendingEvents(); len(got) != 0 {
		t.Errorf("Delete changed the device it was called on: %v", got)
	}
}

// TestDevice_RejectsInvalidValueObjects: the zero values of the name and of
// the protocol are invalid, and no constructor or change method takes them.
func TestDevice_RejectsInvalidValueObjects(t *testing.T) {
	anID := id.NewID[device.Device]()
	name := deviceName(t, "sim-1")

	for _, tc := range map[string]struct {
		name     device.DeviceName
		protocol device.DeviceProtocol
	}{
		"no name":     {protocol: device.ProtocolSimulation},
		"no protocol": {name: name},
	} {
		if _, err := device.CreateDevice(anID, testProject, tc.name, "", tc.protocol, device.EmptyDeviceConfig()); err == nil {
			t.Errorf("%s: CreateDevice took an invalid value object", tc.name)
		}
		if _, err := device.ReconstituteDevice(anID, testProject, tc.name, "", tc.protocol, device.EmptyDeviceConfig(),
			version.Committed[device.Device]()); err == nil {
			t.Errorf("%s: ReconstituteDevice took an invalid value object", tc.name)
		}
	}

	stored, err := device.ReconstituteDevice(anID, testProject, name, "", device.ProtocolSimulation,
		device.EmptyDeviceConfig(), version.Committed[device.Device]())
	if err != nil {
		t.Fatalf("ReconstituteDevice: %v", err)
	}
	if _, err := stored.Update(stored.Version(), device.DeviceName{}, "", device.ProtocolSimulation, device.EmptyDeviceConfig()); err == nil {
		t.Error("Update took an invalid name")
	}
	if _, err := stored.Update(stored.Version(), name, "", device.DeviceProtocol{}, device.EmptyDeviceConfig()); err == nil {
		t.Error("Update took an invalid protocol")
	}
	if got := stored.PendingEvents(); len(got) != 0 {
		t.Errorf("a rejected Update recorded %v", got)
	}
}

func TestDeviceConfig_ZeroValueIsTheEmptyObject(t *testing.T) {
	var zero device.DeviceConfig
	if got := zero.String(); got != `{}` {
		t.Errorf("the zero config is %s, want the empty object", got)
	}
	if got := string(zero.Bytes()); got != `{}` {
		t.Errorf("the zero config's bytes are %s, want the empty object", got)
	}
}

func TestDeviceConfig_IsCopiedInAndOut(t *testing.T) {
	raw := []byte(`{"a":1}`)
	cfg, err := device.NewDeviceConfig(raw)
	if err != nil {
		t.Fatalf("NewDeviceConfig: %v", err)
	}
	raw[2] = 'b'

	out := cfg.Bytes()
	out[2] = 'c'

	if got := cfg.String(); got != `{"a":1}` {
		t.Errorf("config = %s: neither the input nor the output is shared", got)
	}
}

func TestDeviceConfig_RequiresAJSONObject(t *testing.T) {
	for _, raw := range []string{`[1]`, `"text"`, `{`, `null`} {
		if _, err := device.NewDeviceConfig([]byte(raw)); err == nil {
			t.Errorf("NewDeviceConfig(%q): expected an error", raw)
		}
	}
	empty, err := device.NewDeviceConfig(nil)
	if err != nil || empty.String() != `{}` {
		t.Errorf("NewDeviceConfig(nil) = %s, %v; want the empty object", empty, err)
	}
}

func TestDeviceNameAndProtocol_AreValidated(t *testing.T) {
	if _, err := device.NewDeviceName(""); err == nil {
		t.Error("an empty Device name is an error")
	}
	if _, err := device.NewEnvironmentName(""); err == nil {
		t.Error("an empty Environment name is an error")
	}
	if _, err := device.NewDeviceProtocol(""); err == nil {
		t.Error("an empty protocol is an error")
	}
	if _, err := device.NewDeviceProtocol("modbus"); err == nil {
		t.Error("an unknown protocol is an error")
	}
	protocol, err := device.NewDeviceProtocol("simulation")
	if err != nil || protocol != device.ProtocolSimulation || protocol.String() != "simulation" {
		t.Errorf("NewDeviceProtocol(simulation) = %s, %v", protocol, err)
	}
	if (device.DeviceProtocol{}).IsValid() {
		t.Error("the zero protocol must be invalid")
	}
}
