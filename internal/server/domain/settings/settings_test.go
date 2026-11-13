package settings_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/settings"
)

var (
	orgID  = id.NewID(id.IDWithUUID[organization.Organization](uuid.MustParse("a0000000-0000-4000-8000-000000000001")))
	first  = uuid.MustParse("d0000000-0000-4000-8000-000000000001")
	second = uuid.MustParse("d0000000-0000-4000-8000-000000000002")
	third  = uuid.MustParse("d0000000-0000-4000-8000-000000000003")
)

// TestNewOrder_NormalizesDuplicatesAndTheZeroID: one order has one
// representation however it was sent — the zero ID is dropped and a repeated
// one keeps its first place (task 81).
func TestNewOrder_NormalizesDuplicatesAndTheZeroID(t *testing.T) {
	order := settings.NewOrder([]uuid.UUID{second, uuid.Nil, first, second, third})
	if got := order.IDs(); len(got) != 3 || got[0] != second || got[1] != first || got[2] != third {
		t.Errorf("IDs() = %v, want [second, first, third]", got)
	}
	if len(order.IDs()) != 3 {
		t.Error("an order of three libraries is empty")
	}
}

// TestOrder_EmptyIsNeverNil: an empty order answers an empty slice, so a DTO or
// a JSON body holds [] and never null.
func TestOrder_EmptyIsNeverNil(t *testing.T) {
	order := settings.NewOrder(nil)
	if ids := order.IDs(); ids == nil || len(ids) != 0 {
		t.Errorf("IDs() = %v, want an empty slice", ids)
	}
	if len(order.IDs()) != 0 {
		t.Error("an order of no library is not empty")
	}
}

// TestOrder_IDsIsACopy: a caller cannot change the order through the slice it
// gets.
func TestOrder_IDsIsACopy(t *testing.T) {
	order := settings.NewOrder([]uuid.UUID{first})
	ids := order.IDs()
	ids[0] = second
	if order.IDs()[0] != first {
		t.Error("the order changed through the slice it handed out")
	}
}

// TestSettings_WithOrderKeepsTheVersionAndTheOwner: a change of a
// setting is the same aggregate at the same Version — the store raises it
// (task 81, ADR 0024).
func TestSettings_WithOrderKeepsTheVersionAndTheOwner(t *testing.T) {
	own := settings.SingleUser()
	empty := settings.Empty(own, orgID)
	if empty.Version().IsCommitted() {
		t.Error("settings nobody wrote are committed")
	}
	if len(empty.Order().IDs()) != 0 {
		t.Error("settings nobody wrote hold an order")
	}

	changed := empty.WithOrder(settings.NewOrder([]uuid.UUID{second, first}))
	if changed.Version() != empty.Version() {
		t.Errorf("the version moved on a change: %s", changed.Version())
	}
	if changed.Owner() != own || changed.OrganizationID() != orgID {
		t.Errorf("the change moved the settings to %s/%s", changed.Owner(), changed.OrganizationID())
	}
	if got := changed.Order().IDs(); len(got) != 2 || got[0] != second || got[1] != first {
		t.Errorf("the order: %v", got)
	}
	if len(empty.Order().IDs()) != 0 {
		t.Error("the change mutated the settings it was made from")
	}
}

// TestSingleUser_IsOneFixedPerson: every call names the same owner, so the
// settings written before authentication existed are the ones the first User
// inherits (task 81, task 49).
func TestSingleUser_IsOneFixedPerson(t *testing.T) {
	if settings.SingleUser() != settings.SingleUser() {
		t.Error("SingleUser is not one person")
	}
	if got := settings.SingleUser().UUID().String(); got != "f0000000-0000-4000-8000-000000000001" {
		t.Errorf("SingleUser = %s, want the ID the ADR names", got)
	}
}
