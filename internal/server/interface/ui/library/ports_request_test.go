package library

import (
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestPortsRequest_IsASnapshot(t *testing.T) {
	ports := []apiv0.InputPort{{Name: "value"}}
	req := portsRequest(ports)

	ports[0].Name = "changed" // the editor changes its ports before the body is sent

	if req[0].Name != "value" {
		t.Errorf("the request follows the editor: %+v", req)
	}
	if portsRequest(nil) == nil {
		t.Error("nil ports are sent as null")
	}
}
