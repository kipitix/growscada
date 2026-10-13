package apiclient

import (
	"testing"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/versiontest"
)

func TestOlderServerWatch(t *testing.T) {
	own := apiv0.SchemaVersion()
	older := versiontest.Older(own)
	oldest := versiontest.Older(older)
	newer := versiontest.Newer(own)
	seen := []contract.SchemaVersion{older, older, own, newer, older, oldest, oldest}

	for repeat, want := range map[Repeat][]contract.SchemaVersion{
		WarnOnce:     {older},
		WarnOnChange: {older, older, oldest}, // again after the server was replaced
	} {
		var warned []contract.SchemaVersion
		w := &olderServerWatch{repeat: repeat, warn: func(server, client contract.SchemaVersion) {
			if client != own {
				t.Errorf("client version %v, want %v", client, own)
			}
			warned = append(warned, server)
		}}
		for _, v := range seen {
			w.observe(v)
		}
		if len(warned) != len(want) {
			t.Fatalf("repeat %d: warned about %v, want %v", repeat, warned, want)
		}
		for i := range want {
			if warned[i] != want[i] {
				t.Errorf("repeat %d: warned about %v, want %v", repeat, warned, want)
			}
		}
	}
}
