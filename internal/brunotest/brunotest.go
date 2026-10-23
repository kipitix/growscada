// Package brunotest checks the Bruno collections of tests/api/bruno_collections
// (OpenCollection YAML) against the routes of the code they call, for tests
// only: it reads each request with the variables of the collection's localhost
// environment substituted and serves it by a ServeMux of those routes.
package brunotest

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Environment is the environment whose variables are substituted: the one the
// collections are run with.
const Environment = "localhost"

// InvalidBodyTag marks a request whose body the server is expected to reject;
// the body check skips it.
const InvalidBodyTag = "invalid-body"

// Request is one request of a collection.
type Request struct {
	File   string // the request's file name in the collection
	Method string
	URL    string
	Body   string // the JSON body, "" if none
	Tags   []string
}

// HasTag reports whether the request carries the Bruno tag.
func (r Request) HasTag(tag string) bool {
	return slices.Contains(r.Tags, tag)
}

// CollectionDir is the directory of the named collection in this repository,
// found from the working directory up (go test runs in the package's
// directory), so that it holds under -trimpath too. Outside the repository it
// is a relative path that Read fails to find.
func CollectionDir(name string) string {
	rel := filepath.Join("tests", "api", "bruno_collections", name)
	dir, err := os.Getwd()
	if err != nil {
		return rel
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, rel)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return rel
		}
		dir = parent
	}
}

// requestFile is the part of a request file the tests read.
type requestFile struct {
	Info struct {
		Tags []string `yaml:"tags"`
	} `yaml:"info"`
	HTTP struct {
		Method string `yaml:"method"`
		URL    string `yaml:"url"`
		Body   struct {
			Type string `yaml:"type"`
			Data string `yaml:"data"`
		} `yaml:"body"`
	} `yaml:"http"`
}

type environmentFile struct {
	Variables []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"variables"`
}

var variable = regexp.MustCompile(`\{\{\s*([^{}\s]+)\s*\}\}`)

// Read returns the requests of the collection in dir, sorted by file name,
// with the variables of its localhost environment substituted. A variable the
// environment does not define stays as it is, as Bruno leaves it, except in a
// URL, where it is an error: there it is a typo, not text. Bruno's dynamic
// variables ({{$randomInt}}) stay as they are everywhere.
func Read(dir string) ([]Request, error) {
	vars, err := readEnvironment(filepath.Join(dir, "environments", Environment+".yml"))
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}
	var requests []Request
	for _, path := range files {
		name := filepath.Base(path)
		if name == "opencollection.yml" {
			continue
		}
		var f requestFile
		if err := readYAML(path, &f); err != nil {
			return nil, err
		}
		if f.HTTP.Body.Type != "" && f.HTTP.Body.Type != "json" {
			return nil, fmt.Errorf("%s: body type %q, only json is supported", name, f.HTTP.Body.Type)
		}
		req := Request{File: name, Method: f.HTTP.Method, Tags: f.Info.Tags}
		if req.URL, err = substitute(f.HTTP.URL, vars, true); err != nil {
			return nil, fmt.Errorf("%s: url: %w", name, err)
		}
		if req.Body, err = substitute(f.HTTP.Body.Data, vars, false); err != nil {
			return nil, fmt.Errorf("%s: body: %w", name, err)
		}
		requests = append(requests, req)
	}
	return requests, nil
}

func readEnvironment(path string) (map[string]string, error) {
	var env environmentFile
	if err := readYAML(path, &env); err != nil {
		return nil, err
	}
	vars := make(map[string]string, len(env.Variables))
	for _, v := range env.Variables {
		vars[v.Name] = strings.TrimSpace(v.Value)
	}
	return vars, nil
}

// substitute replaces the environment's variables in s; strict makes an
// undefined one (but a dynamic $variable) an error.
func substitute(s string, vars map[string]string, strict bool) (string, error) {
	var missing []string
	out := variable.ReplaceAllStringFunc(s, func(m string) string {
		name := variable.FindStringSubmatch(m)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		if strict && !strings.HasPrefix(name, "$") {
			missing = append(missing, name)
		}
		return m
	})
	if missing != nil {
		return "", fmt.Errorf("variables not in the %s environment: %s", Environment, strings.Join(missing, ", "))
	}
	return out, nil
}

func readYAML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

// Collection is what a collection is checked against: the patterns of the
// routes it calls and how their request bodies are decoded.
type Collection struct {
	Dir      string
	Patterns []string
	// Bodies maps the pattern of each route that takes a body to a value of
	// the body's type. Every POST/PUT/PATCH route needs an entry: nil for one
	// that takes no body.
	Bodies map[string]any
	// Decode decodes a body as the handlers do.
	Decode func(body io.Reader, v any) error
}

// Check reports every request of the collection that no route serves, every
// route no request covers, every body Decode rejects (except in requests
// tagged InvalidBodyTag) and every entry of Bodies for no route.
func Check(t *testing.T, c Collection) {
	t.Helper()
	problems, err := c.problems()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// patternHeader carries, in the response of the ServeMux built by problems,
// the pattern of the route that served the request.
const patternHeader = "Brunotest-Pattern"

func (c Collection) problems() ([]string, error) {
	requests, err := Read(c.Dir)
	if err != nil {
		return nil, err
	}
	// A route is served by the handler registered for it, never by the
	// redirect ServeMux answers with for an unclean path or a missing
	// trailing slash: that request is served by no route.
	mux := http.NewServeMux()
	for _, pattern := range c.Patterns {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(patternHeader, r.Pattern)
		})
	}
	var problems []string
	covered := make(map[string]bool)
	for _, r := range requests {
		req, err := http.NewRequest(r.Method, r.URL, nil)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", r.File, err))
			continue
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		pattern := rec.Header().Get(patternHeader)
		if pattern == "" {
			problems = append(problems, fmt.Sprintf("%s: %s %s is served by no route (the mux answers %d)",
				r.File, r.Method, req.URL.Path, rec.Code))
			continue
		}
		covered[pattern] = true
		if !r.HasTag(InvalidBodyTag) {
			problems = append(problems, c.bodyProblems(r, pattern)...)
		}
	}
	for _, pattern := range c.Patterns {
		if !covered[pattern] {
			problems = append(problems, fmt.Sprintf("route %q is covered by no request of the collection", pattern))
		}
		if _, known := c.Bodies[pattern]; !known && takesBody(pattern) {
			problems = append(problems, fmt.Sprintf(
				"add the body type of route %q to the test's requestBodies (nil if it takes none)", pattern))
		}
	}
	for _, pattern := range slices.Sorted(maps.Keys(c.Bodies)) {
		if !slices.Contains(c.Patterns, pattern) {
			problems = append(problems, fmt.Sprintf("requestBodies has %q, which is no route: remove it", pattern))
		}
	}
	return problems, nil
}

func (c Collection) bodyProblems(r Request, pattern string) []string {
	body := c.Bodies[pattern]
	switch {
	case body == nil && r.Body != "":
		return []string{fmt.Sprintf("%s: route %q takes no body, the request has one", r.File, pattern)}
	case body != nil && r.Body == "":
		return []string{fmt.Sprintf("%s: route %q takes a body, the request has none", r.File, pattern)}
	case body != nil:
		v := reflect.New(reflect.TypeOf(body)).Interface()
		if err := c.Decode(strings.NewReader(r.Body), v); err != nil {
			return []string{fmt.Sprintf("%s: body is not a %T: %v (tag the request %q if it is meant to be rejected)",
				r.File, body, err, InvalidBodyTag)}
		}
	}
	return nil
}

// takesBody reports whether the pattern's method usually carries a body: such
// a route needs an entry in Collection.Bodies.
func takesBody(pattern string) bool {
	method, _, _ := strings.Cut(pattern, " ")
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
}
