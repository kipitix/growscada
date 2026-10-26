package migrations

import (
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// defaultTable is goose's own table, the one a service names no -table for.
const defaultTable = "goose_db_version"

// composeFiles are the docker compose files that apply the chains.
var composeFiles = []string{
	"../../../../../tools/debug_db/docker-compose.yaml",
	"../../../../../tests/api/docker-compose.yaml",
}

type composeFile struct {
	Services map[string]struct {
		Volumes     []string          `yaml:"volumes"`
		Environment map[string]string `yaml:"environment"`
	} `yaml:"services"`
}

// TestComposeFilesApplyEveryChain: each compose file has a goose service per
// chain, mounting its directory and recording it in its table.
func TestComposeFilesApplyEveryChain(t *testing.T) {
	for _, path := range composeFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var compose composeFile
		if err := yaml.Unmarshal(data, &compose); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, c := range chains {
			mount := "/migrations/" + c.Dir + ":/migrations"
			found := false
			for name, service := range compose.Services {
				if !slices.ContainsFunc(service.Volumes, func(v string) bool { return strings.Contains(v, mount) }) {
					continue
				}
				found = true
				var table string
				for _, arg := range strings.Fields(service.Environment["GOOSE_EXTRA_ARGS"]) {
					if v, ok := strings.CutPrefix(arg, "-table="); ok {
						table = v
					}
				}
				if table == "" {
					table = defaultTable
				}
				if table != c.Table {
					t.Errorf("%s: service %s records chain %s in %s, want %s", path, name, c.Dir, table, c.Table)
				}
			}
			if !found {
				t.Errorf("%s: no service mounts chain %s", path, c.Dir)
			}
		}
	}
}
