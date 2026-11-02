package device_test

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/device"
)

func TestNewDeviceName_ReturnsCorrectValue(t *testing.T) {
	name, err := device.NewDeviceName("sim-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := name.String(); got != "sim-1" {
		t.Errorf("expected %q, got %q", "sim-1", got)
	}
	if !name.IsValid() {
		t.Error("a name created by NewDeviceName is valid")
	}
}

func TestNewDeviceName_EmptyString(t *testing.T) {
	if _, err := device.NewDeviceName(""); err == nil {
		t.Error("expected an error for an empty name, got nil")
	}
}

// TestDeviceName_IsTheUniqueKey: within its Project a Device's name is its
// unique natural key (ADR 0003), so two names are one name exactly when their
// strings are equal — what the repository's uniqueness check keys on.
func TestDeviceName_IsTheUniqueKey(t *testing.T) {
	first := deviceName(t, "sim-1")
	second := deviceName(t, "sim-1")
	other := deviceName(t, "sim-2")

	if first != second {
		t.Errorf("%q and %q are the same name", first, second)
	}
	if first == other {
		t.Errorf("%q and %q are different names", first, other)
	}
	var zero device.DeviceName
	if zero.IsValid() {
		t.Error("the zero name is invalid")
	}
}
