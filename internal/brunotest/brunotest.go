// Package brunotest checks the Bruno collections of tests/api/bruno_collections
// (OpenCollection YAML) against the routes of the code they call, for tests
// only: it reads each request with the variables of the collection's localhost
// environment substituted and matches it against a ServeMux.
package brunotest

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
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

// CollectionDir is the directory of the named collection in this repository.
func CollectionDir(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "tests", "api", "bruno_collections", name)
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
// with the variables of its localhost environment substituted. A variable
// the environment does not define is an error.
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
		if req.URL, err = substitute(f.HTTP.URL, vars); err != nil {
			return nil, fmt.Errorf("%s: url: %w", name, err)
		}
		if req.Body, err = substitute(f.HTTP.Body.Data, vars); err != nil {
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

func substitute(s string, vars map[string]string) (string, error) {
	var missing []string
	out := variable.ReplaceAllStringFunc(s, func(m string) string {
		name := variable.FindStringSubmatch(m)[1]
		v, ok := vars[name]
		if !ok {
			missing = append(missing, name)
		}
		return v
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

// Collection is what a collection is checked against: the routes of a
// ServeMux and how their request bodies are decoded.
type Collection struct {
	Dir string
	// Mux serves Patterns, the patterns of every route.
	Mux      *http.ServeMux
	Patterns []string
	// Bodies maps the pattern of each route that takes a body to a value of
	// the body's type. Every POST/PUT/PATCH route needs an entry: nil for one
	// that takes no body.
	Bodies map[string]any
	// Decode decodes a body as the handlers do.
	Decode func(body io.Reader, v any) error
}

// Check reports every request of the collection that no route serves, every
// route no request covers and every body Decode rejects (except in requests
// tagged InvalidBodyTag).
func Check(t *testing.T, c Collection) {
	t.Helper()
	requests, err := Read(c.Dir)
	if err != nil {
		t.Fatal(err)
	}
	covered := make(map[string]bool)
	for _, r := range requests {
		req, err := http.NewRequest(r.Method, r.URL, nil)
		if err != nil {
			t.Errorf("%s: %v", r.File, err)
			continue
		}
		_, pattern := c.Mux.Handler(req)
		if pattern == "" {
			t.Errorf("%s: %s %s is served by no route", r.File, r.Method, req.URL.Path)
			continue
		}
		covered[pattern] = true
		if !r.HasTag(InvalidBodyTag) {
			checkBody(t, c, r, pattern)
		}
	}
	for _, pattern := range c.Patterns {
		if !covered[pattern] {
			t.Errorf("route %q is covered by no request of the collection", pattern)
		}
		if _, known := c.Bodies[pattern]; !known && takesBody(pattern) {
			t.Errorf("add the body type of route %q to the test's requestBodies (nil if it takes none)", pattern)
		}
	}
}

func checkBody(t *testing.T, c Collection, r Request, pattern string) {
	t.Helper()
	body := c.Bodies[pattern]
	switch {
	case body == nil && r.Body != "":
		t.Errorf("%s: route %q takes no body, the request has one", r.File, pattern)
	case body != nil && r.Body == "":
		t.Errorf("%s: route %q takes a body, the request has none", r.File, pattern)
	case body != nil:
		v := reflect.New(reflect.TypeOf(body)).Interface()
		if err := c.Decode(strings.NewReader(r.Body), v); err != nil {
			t.Errorf("%s: body is not a %T: %v (tag the request %q if it is meant to be rejected)",
				r.File, body, err, InvalidBodyTag)
		}
	}
}

// takesBody reports whether the pattern's method usually carries a body: such
// a route needs an entry in Collection.Bodies.
func takesBody(pattern string) bool {
	method, _, _ := strings.Cut(pattern, " ")
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
}
