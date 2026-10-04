package restapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

type decodeTarget struct {
	Name  string `json:"name"`
	Inner struct {
		Value int `json:"value"`
	} `json:"inner"`
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
}

// decode runs decodeRequest on body as a client of the given SchemaVersion and
// returns whether it succeeded and the problem detail.
func decode(t *testing.T, body, clientVersion string) (bool, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	if clientVersion != "" {
		req.Header.Set(contract.SchemaVersionHeader, clientVersion)
	}
	rec := httptest.NewRecorder()
	var v decodeTarget
	if decodeRequest(rec, req, &v) {
		return true, ""
	}
	var prob ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return false, prob.Detail
}

func TestDecodeRequest_AcceptsWellFormedBody(t *testing.T) {
	body := `{"name":"a","inner":{"value":1},"items":[{"id":"x"},{"id":"y"}]}`
	if ok, detail := decode(t, body, ""); !ok {
		t.Fatalf("rejected: %s", detail)
	}
}

func TestDecodeRequest_RejectsWhatWouldBeDropped(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate key":          `{"name":"a","name":"b"}`,
		"duplicate key, case":    `{"name":"a","Name":"b"}`,
		"duplicate nested key":   `{"inner":{"value":1,"value":2}}`,
		"duplicate key in array": `{"items":[{"id":"x","ID":"y"}]}`,
		"trailing value":         `{"name":"a"}{"name":"b"}`,
		"trailing garbage":       `{"name":"a"} x`,
	} {
		if ok, _ := decode(t, body, ""); ok {
			t.Errorf("%s: %s accepted", name, body)
		}
	}
}

// Only an unknown field can mean the client is newer than the server; any
// other error is the client's own and must not blame the server version.
func TestDecodeRequest_NewerClient_OtherErrors_DoNotSayUpdateServer(t *testing.T) {
	newer := contract.SchemaVersion{Major: apiv0.SchemaVersion().Major, Minor: apiv0.SchemaVersion().Minor + 1}
	for name, body := range map[string]string{
		"syntax":     `{"name":`,
		"wrong type": `{"name":1}`,
		"duplicate":  `{"name":"a","name":"b"}`,
		"trailing":   `{"name":"a"}{}`,
	} {
		ok, detail := decode(t, body, newer.String())
		if ok {
			t.Errorf("%s: accepted", name)
			continue
		}
		if strings.Contains(detail, "update the server") {
			t.Errorf("%s: detail %q blames the server version", name, detail)
		}
	}
	if _, detail := decode(t, `{"unit":"rpm"}`, newer.String()); !strings.Contains(detail, "update the server") {
		t.Errorf("unknown field: detail %q does not say to update the server", detail)
	}
}
