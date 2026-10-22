package brunotest

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeCollection(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "environments"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const environment = `name: localhost
variables:
  - name: HOST
    value: |
      localhost:9090
  - name: ID
    value: 42
`

func TestRead_SubstitutesTheEnvironment(t *testing.T) {
	dir := writeCollection(t, map[string]string{
		"environments/localhost.yml": environment,
		"opencollection.yml":         "opencollection: 1.0.0\n",
		"post_item.yml": `info:
  name: post item
  seq: 1
  tags:
    - invalid-body
http:
  method: POST
  url: http://{{HOST}}/items/{{ ID }}
  body:
    type: json
    data: '{"id": "{{ID}}"}'
`,
	})
	requests, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := Request{File: "post_item.yml", Method: "POST", URL: "http://localhost:9090/items/42",
		Body: `{"id": "42"}`, Tags: []string{InvalidBodyTag}}
	if len(requests) != 1 || !requestsEqual(requests[0], want) {
		t.Fatalf("Read = %+v, want [%+v]", requests, want)
	}
}

func TestRead_UnknownVariable_IsAnError(t *testing.T) {
	dir := writeCollection(t, map[string]string{
		"environments/localhost.yml": environment,
		"get_item.yml": `info:
  name: get item
http:
  method: GET
  url: http://{{HOST}}/items/{{ITEM_ID}}
`,
	})
	_, err := Read(dir)
	if err == nil || !strings.Contains(err.Error(), "get_item.yml") || !strings.Contains(err.Error(), "ITEM_ID") {
		t.Fatalf("Read error = %v, want one naming get_item.yml and ITEM_ID", err)
	}
}

func requestsEqual(a, b Request) bool {
	return a.File == b.File && a.Method == b.Method && a.URL == b.URL && a.Body == b.Body && slices.Equal(a.Tags, b.Tags)
}
