package event_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/kipitix/growscada/internal/domain/event"
)

// TestAllEventTypes_ListsEveryDeclaredEventType guards against declaring an
// EventType value and forgetting to add it to eventTypeNames: such a value
// would be missing from AllEventTypes (and thus never reach SSE clients)
// and would print as "invalid".
func TestAllEventTypes_ListsEveryDeclaredEventType(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "event_type.go", nil, 0)
	if err != nil {
		t.Fatalf("parse event_type.go: %v", err)
	}

	var declared []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec := spec.(*ast.ValueSpec)
			for i, name := range valueSpec.Names {
				if !name.IsExported() || i >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[i].(*ast.CompositeLit)
				if !ok {
					continue
				}
				if typ, ok := lit.Type.(*ast.Ident); ok && typ.Name == "EventType" {
					declared = append(declared, name.Name)
				}
			}
		}
	}

	if len(declared) == 0 {
		t.Fatal("found no EventType declarations in event_type.go")
	}
	if got := len(event.AllEventTypes()); got != len(declared) {
		t.Errorf("AllEventTypes has %d values, but event_type.go declares %d: %v",
			got, len(declared), declared)
	}
}
