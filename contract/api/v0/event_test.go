package apiv0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestEventTypes_ListsEveryEventConstant guards EventTypes from the constants'
// side: an Event… name declared but not listed would be published by the
// server yet unknown to everything that ranges over EventTypes.
func TestEventTypes_ListsEveryEventConstant(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "event.go", nil, 0)
	if err != nil {
		t.Fatalf("parse event.go: %v", err)
	}
	var declared []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Event") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("%s is not a string literal", name.Name)
				}
				value, _ := strconv.Unquote(lit.Value)
				declared = append(declared, value)
			}
		}
	}

	listed := EventTypes()
	slices.Sort(declared)
	slices.Sort(listed)
	if !slices.Equal(declared, listed) {
		t.Errorf("EventTypes() %v, want every Event constant %v", listed, declared)
	}
	if len(slices.Compact(listed)) != len(EventTypes()) {
		t.Errorf("EventTypes() lists a type twice: %v", EventTypes())
	}
}
