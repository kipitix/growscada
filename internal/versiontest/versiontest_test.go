package versiontest_test

import (
	"testing"

	"github.com/kipitix/growscada/contract"
	"github.com/kipitix/growscada/internal/versiontest"
)

func TestOlderAndNewer_AreValidAndOrdered(t *testing.T) {
	for _, v := range []contract.SchemaVersion{{Major: 0, Minor: 2}, {Major: 1, Minor: 0}, {Major: 2, Minor: 5}} {
		older, newer := versiontest.Older(v), versiontest.Newer(v)
		if !v.NewerThan(older) || !newer.NewerThan(v) {
			t.Errorf("%v: older %v, newer %v", v, older, newer)
		}
		for _, w := range []contract.SchemaVersion{older, newer, versiontest.Older(older)} {
			if _, err := contract.ParseSchemaVersion(w.String()); err != nil {
				t.Errorf("%v: %v", v, err)
			}
		}
	}
}
