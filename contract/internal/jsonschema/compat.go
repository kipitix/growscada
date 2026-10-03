package jsonschema

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
)

// MinorIncompatibilities lists why a newer MINOR schema is not a compatible
// extension of the older one of the same MAJOR, or nil if it is. Both are
// whole generated documents. A compatible MINOR keeps every type and
// property of the old one unchanged, keeps the same required properties,
// and may only add types and optional properties.
func MinorIncompatibilities(older, newer map[string]any) []string {
	var problems []string
	oldDefs, _ := older["$defs"].(map[string]any)
	newDefs, _ := newer["$defs"].(map[string]any)
	for _, name := range sortedKeys(oldDefs) {
		newDef, ok := newDefs[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("$defs/%s: removed", name))
			continue
		}
		compareNode("$defs/"+name, oldDefs[name], newDef, &problems)
	}
	return problems
}

func compareNode(path string, older, newer any, problems *[]string) {
	oldMap, oldIsMap := older.(map[string]any)
	newMap, newIsMap := newer.(map[string]any)
	if !oldIsMap || !newIsMap {
		if !reflect.DeepEqual(older, newer) {
			*problems = append(*problems, fmt.Sprintf("%s: changed", path))
		}
		return
	}

	keys := map[string]bool{}
	for k := range oldMap {
		keys[k] = true
	}
	for k := range newMap {
		keys[k] = true
	}
	for _, key := range sortedKeys(keys) {
		switch key {
		case "properties":
			compareProperties(path, oldMap, newMap, problems)
		case "required":
			if !sameStringSet(oldMap[key], newMap[key]) {
				*problems = append(*problems, fmt.Sprintf("%s: required properties changed", path))
			}
		case "items", "additionalProperties":
			compareNode(path+"/"+key, oldMap[key], newMap[key], problems)
		default:
			if !reflect.DeepEqual(oldMap[key], newMap[key]) {
				*problems = append(*problems, fmt.Sprintf("%s: %s changed", path, key))
			}
		}
	}
}

func compareProperties(path string, older, newer map[string]any, problems *[]string) {
	oldProps, _ := older["properties"].(map[string]any)
	newProps, _ := newer["properties"].(map[string]any)
	for _, name := range sortedKeys(oldProps) {
		newProp, ok := newProps[name]
		if !ok {
			*problems = append(*problems, fmt.Sprintf("%s/%s: removed", path, name))
			continue
		}
		compareNode(path+"/"+name, oldProps[name], newProp, problems)
	}
	// Added properties are fine; making them required is caught by the
	// required-set comparison.
}

func sameStringSet(a, b any) bool {
	as, bs := stringList(a), stringList(b)
	slices.Sort(as)
	slices.Sort(bs)
	return slices.Equal(as, bs)
}

func stringList(v any) []string {
	var out []string
	switch list := v.(type) {
	case []any:
		for _, item := range list {
			out = append(out, fmt.Sprint(item))
		}
	case []string:
		out = append(out, list...)
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
