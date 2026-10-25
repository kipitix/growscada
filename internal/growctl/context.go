package growctl

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/kipitix/growscada/internal/server/domain/project"
)

// ConfigEnv is the environment variable holding the path of growctl's
// context file; by default it is growctl/config.yaml in the user's
// configuration directory.
const ConfigEnv = "GROWCTL_CONFIG"

// growctlContext is what the context file keeps between runs, like a
// kubectl context: the project growctl works in when --project is not given.
type growctlContext struct {
	Project string `yaml:"project,omitempty"`
}

// contextPath returns the path of the context file.
func contextPath() (string, error) {
	if path := os.Getenv(ConfigEnv); path != "" {
		return path, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot find the context file (set %s): %w", ConfigEnv, err)
	}
	return filepath.Join(dir, "growctl", "config.yaml"), nil
}

// readContext reads the context file; a missing one is an empty context.
func readContext() (growctlContext, error) {
	path, err := contextPath()
	if err != nil {
		return growctlContext{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return growctlContext{}, nil
	}
	if err != nil {
		return growctlContext{}, fmt.Errorf("cannot read the context: %w", err)
	}
	var c growctlContext
	if err := yaml.Unmarshal(data, &c); err != nil {
		return growctlContext{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// writeContext replaces the context file, creating its directory.
func writeContext(c growctlContext) (string, error) {
	path, err := contextPath()
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("cannot write the context: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("cannot write the context: %w", err)
	}
	return path, nil
}

// errNoProject is returned when no project is named anywhere.
var errNoProject = errors.New("no project: use --project, growctl config use-project <name> or metadata.project")

// projectFor returns the project growctl works in (ADR 0006): --project
// (flag), else the context's, else the one the Tag manifests name in
// metadata.project. A Tag manifest naming another project is an error, and
// so is no project at all (a usage error).
func projectFor(flag string, tags []TagManifest) (string, error) {
	name, from := flag, "--project"
	if name == "" {
		c, err := readContext()
		if err != nil {
			return "", err
		}
		name, from = c.Project, "the context"
	}
	for _, t := range tags {
		switch {
		case t.Project == "":
		case name == "":
			name, from = t.Project, "metadata.project of tag/"+t.Name
		case t.Project != name:
			return "", fmt.Errorf("tag/%s: metadata.project is %q, but growctl works in project %q (from %s)",
				t.Name, t.Project, name, from)
		}
	}
	if name == "" {
		return "", usageError(errNoProject)
	}
	if _, err := project.NewProjectName(name); err != nil {
		return "", usageError(fmt.Errorf("invalid project %q: %w", name, err))
	}
	return name, nil
}
