// Package server_test checks the boundary between the roles of the server
// (ADR 0011): the Engineering node and the Runtime node share the domain's
// value objects and the server's public schema, never each other's code or
// schema. The Engineering node reaches the Runtime node only through its
// RuntimeNode port; what wires the two in one process (inprocess, main.go,
// servertest) and the interface layer serving both are no role's.
package server_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/kipitix/growscada/"

// runtimeRole are the packages of the Runtime node.
var runtimeRole = []string{
	"internal/server/domain/tagstate",
	"internal/server/application/runtime",
	"internal/server/infrastructure/postgres/runtime",
}

// engineeringRole are the packages of the Engineering node (with the outbox,
// which carries its events).
var engineeringRole = []string{
	"internal/server/domain/scene",
	"internal/server/domain/library",
	"internal/server/application",
	"internal/server/application/appdto",
	"internal/server/infrastructure/postgres/repositories",
	"internal/server/infrastructure/postgres/outbox",
}

// sharedDomain are the packages of the domain both roles build on: the Tag's
// value objects, IDs, events, Projects and their Organization.
var sharedDomain = []string{
	"internal/server/domain/tag",
	"internal/server/domain/project",
	"internal/server/domain/organization",
	"internal/server/domain/id",
	"internal/server/domain/event",
	"internal/server/domain/version",
}

type listedPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	Deps       []string
}

// listPackages returns the packages of the patterns with their transitive
// dependencies (tests' own imports left out).
func listPackages(t *testing.T, patterns ...string) map[string]listedPackage {
	t.Helper()
	args := append([]string{"list", "-json"}, patterns...)
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	listed := map[string]listedPackage{}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p listedPackage
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decode go list: %v", err)
		}
		listed[strings.TrimPrefix(p.ImportPath, module)] = p
	}
	return listed
}

func patterns(paths ...[]string) []string {
	var all []string
	for _, ps := range paths {
		for _, p := range ps {
			all = append(all, module+p)
		}
	}
	return all
}

func isIn(path string, role []string) bool {
	for _, p := range role {
		if path == p {
			return true
		}
	}
	return false
}

func TestRoles_DoNotImportEachOther(t *testing.T) {
	listed := listPackages(t, patterns(runtimeRole, engineeringRole)...)
	if len(listed) != len(runtimeRole)+len(engineeringRole) {
		t.Fatalf("listed %d packages, want every package of the roles: %v", len(listed), listed)
	}
	for path, p := range listed {
		for _, dep := range p.Deps {
			dep, ours := strings.CutPrefix(dep, module)
			if !ours {
				continue
			}
			switch {
			case isIn(path, engineeringRole) && isIn(dep, runtimeRole):
				t.Errorf("%s (Engineering node) imports %s (Runtime node): reach it through application.RuntimeNode", path, dep)
			case isIn(path, runtimeRole) && !isIn(dep, runtimeRole) && !isIn(dep, sharedDomain):
				t.Errorf("%s (Runtime node) imports %s, neither the Runtime node's nor the shared domain", path, dep)
			}
		}
	}
}

// engineeringTable and runtimeTable match a table of the role's schema
// named in an SQL string.
var (
	engineeringTable = regexp.MustCompile(`\bengineering\.[a-z_]+`)
	runtimeTable     = regexp.MustCompile(`\bruntime\.[a-z_]+`)
)

// TestRoles_QueryNoSchemaOfTheOther: no string of a role's code names a
// table of the other role's schema (ADR 0011); both may name public's.
func TestRoles_QueryNoSchemaOfTheOther(t *testing.T) {
	listed := listPackages(t, patterns(runtimeRole, engineeringRole)...)
	for path, p := range listed {
		forbidden := runtimeTable
		if isIn(path, runtimeRole) {
			forbidden = engineeringTable
		}
		for _, name := range p.GoFiles {
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(p.Dir, name), nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				text, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				if found := forbidden.FindString(text); found != "" {
					t.Errorf("%s/%s names %s, a table of the other role's schema", path, name, found)
				}
				return true
			})
		}
	}
}
