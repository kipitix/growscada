package device_test

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/device"
)

func TestNewEnvironmentName_ReturnsCorrectValue(t *testing.T) {
	name, err := device.NewEnvironmentName("test bench")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := name.String(); got != "test bench" {
		t.Errorf("expected %q, got %q", "test bench", got)
	}
	if !name.IsValid() {
		t.Error("a name created by NewEnvironmentName is valid")
	}
}

func TestNewEnvironmentName_EmptyString(t *testing.T) {
	if _, err := device.NewEnvironmentName(""); err == nil {
		t.Error("expected an error for an empty name, got nil")
	}
}

// TestEnvironmentName_IsTheUniqueKey: within its Project an Environment's
// name is its unique natural key — a Gateway names the Environment it serves,
// and the name is resolved to it — so two names are one name exactly when
// their strings are equal.
func TestEnvironmentName_IsTheUniqueKey(t *testing.T) {
	first := environmentName(t, "plant")
	second := environmentName(t, "plant")
	other := environmentName(t, "test bench")

	if first != second {
		t.Errorf("%q and %q are the same name", first, second)
	}
	if first == other {
		t.Errorf("%q and %q are different names", first, other)
	}
	var zero device.EnvironmentName
	if zero.IsValid() {
		t.Error("the zero name is invalid")
	}
}
