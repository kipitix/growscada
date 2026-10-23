package brunotest

import (
	"encoding/json"
	"io"
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

func TestRead_BodyKeepsUndefinedAndDynamicVariables(t *testing.T) {
	dir := writeCollection(t, map[string]string{
		"environments/localhost.yml": environment,
		"post_item.yml": `info:
  name: post item
http:
  method: POST
  url: http://{{HOST}}/items/{{$randomInt}}
  body:
    type: json
    data: '{"html": "<b>{{value}}</b>", "n": "{{$randomInt}}", "id": "{{ID}}"}'
`,
	})
	requests, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 {
		t.Fatalf("Read = %+v, want one request", requests)
	}
	if want := "http://localhost:9090/items/{{$randomInt}}"; requests[0].URL != want {
		t.Errorf("URL = %q, want %q", requests[0].URL, want)
	}
	if want := `{"html": "<b>{{value}}</b>", "n": "{{$randomInt}}", "id": "42"}`; requests[0].Body != want {
		t.Errorf("Body = %q, want %q", requests[0].Body, want)
	}
}

type itemBody struct {
	Name string `json:"name"`
}

func decodeStrict(body io.Reader, v any) error {
	d := json.NewDecoder(body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func requestYAML(method, path, body string) string {
	f := "info:\n  name: r\nhttp:\n  method: " + method + "\n  url: http://{{HOST}}" + path + "\n"
	if body != "" {
		f += "  body:\n    type: json\n    data: '" + body + "'\n"
	}
	return f
}

func TestCheck_AMatchingCollection_HasNoProblems(t *testing.T) {
	c := Collection{
		Dir: writeCollection(t, map[string]string{
			"environments/localhost.yml": environment,
			"get_item.yml":               requestYAML("GET", "/items/{{ID}}", ""),
			"post_items.yml":             requestYAML("POST", "/items", `{"name": "a"}`),
		}),
		Patterns: []string{"GET /items/{id}", "POST /items"},
		Bodies:   map[string]any{"POST /items": itemBody{}},
		Decode:   decodeStrict,
	}
	problems, err := c.problems()
	if err != nil || len(problems) != 0 {
		t.Fatalf("problems = %q, %v; want none", problems, err)
	}
}

func TestCheck_AnswerByRedirect_IsServedByNoRoute(t *testing.T) {
	for name, path := range map[string]string{
		"unclean path":           "//items/{{ID}}",
		"missing trailing slash": "/dir",
	} {
		t.Run(name, func(t *testing.T) {
			c := Collection{
				Dir: writeCollection(t, map[string]string{
					"environments/localhost.yml": environment,
					"get.yml":                    requestYAML("GET", path, ""),
				}),
				Patterns: []string{"GET /items/{id}", "GET /dir/"},
				Decode:   decodeStrict,
			}
			problems, err := c.problems()
			if err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(problems, func(p string) bool {
				return strings.HasPrefix(p, "get.yml:") && strings.Contains(p, "served by no route")
			}) {
				t.Fatalf("problems = %q, want get.yml served by no route", problems)
			}
		})
	}
}

func TestCheck_BodiesEntryForNoRoute_IsAProblem(t *testing.T) {
	c := Collection{
		Dir: writeCollection(t, map[string]string{
			"environments/localhost.yml": environment,
			"post_items.yml":             requestYAML("POST", "/items", `{"name": "a"}`),
		}),
		Patterns: []string{"POST /items"},
		Bodies:   map[string]any{"POST /items": itemBody{}, "PUT /items/{id}": itemBody{}},
		Decode:   decodeStrict,
	}
	problems, err := c.problems()
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], `"PUT /items/{id}", which is no route`) {
		t.Fatalf("problems = %q, want one about PUT /items/{id}", problems)
	}
}

func requestsEqual(a, b Request) bool {
	return a.File == b.File && a.Method == b.Method && a.URL == b.URL && a.Body == b.Body && slices.Equal(a.Tags, b.Tags)
}
