package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kipitix/growscada/contract/internal/jsonschema"
)

// TestCommittedMinorsAreCompatible checks every pair of consecutive committed
// schemas of the same MAJOR: the newer MINOR may only add what old readers
// can ignore. MAJOR 0 promises no compatibility: its pairs are still checked,
// so the check runs on real schemas long before 1.0, but only logged.
func TestCommittedMinorsAreCompatible(t *testing.T) {
	contracts, err := os.ReadDir("../schemas")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range contracts {
		if !dir.IsDir() {
			continue
		}
		versions := committedVersions(t, filepath.Join("../schemas", dir.Name()))
		for i := 1; i < len(versions); i++ {
			older, newer := versions[i-1], versions[i]
			if !newer.SameMajor(older) {
				continue
			}
			t.Run(dir.Name()+"/"+newer.String(), func(t *testing.T) {
				problems := jsonschema.MinorIncompatibilities(
					readSchema(t, dir.Name(), older), readSchema(t, dir.Name(), newer))
				if len(problems) > 0 && newer.Major == 0 {
					t.Logf("%s %v breaks readers of %v (allowed while MAJOR is 0):\n%s",
						dir.Name(), newer, older, strings.Join(problems, "\n"))
					return
				}
				if len(problems) > 0 {
					t.Fatalf("%s %v is not a compatible MINOR of %v — raise MAJOR instead:\n%s",
						dir.Name(), newer, older, strings.Join(problems, "\n"))
				}
			})
		}
	}
}

func committedVersions(t *testing.T, dir string) []SchemaVersion {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var versions []SchemaVersion
	for _, f := range files {
		name, ok := strings.CutSuffix(f.Name(), ".json")
		if !ok {
			continue
		}
		v, err := ParseSchemaVersion(name)
		if err != nil {
			t.Fatalf("%s/%s: %v", dir, f.Name(), err)
		}
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[j].NewerThan(versions[i]) })
	return versions
}

func readSchema(t *testing.T, contract string, v SchemaVersion) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../schemas", contract, v.String()+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
