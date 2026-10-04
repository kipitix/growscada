package jsonschema

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// A contract's schemas, once published, never change (ADR 0005): a reader
// that validated against schemas/api/0.1.json must keep getting that format.
// Two guards keep it so:
//
//   - every version older than the current one is frozen: its SHA-256 is
//     recorded in frozenFile next to it (sha256sum format, so `sha256sum -c`
//     works too), and the tests fail when the file no longer matches;
//   - make schemas refuses to rewrite the current version when that version
//     is already on the published branch, which is what happens when the
//     types change and the SchemaVersion is not raised.

// frozenFile lists the hashes of a contract's frozen schemas.
const frozenFile = "frozen.sha256"

// errUnchecked means the published branch could not be consulted: no git, or
// no such branch (a shallow clone). make schemas then warns and goes on.
var errUnchecked = errors.New("cannot read the published branch")

// publishedSchema returns the content of the schema file at path on the git
// ref, or nil when the ref does not have it (the version is new).
func publishedSchema(ref, path string) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(abs)
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUnchecked, err)
	}
	if _, err := git(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return nil, fmt.Errorf("%w: no branch %q", errUnchecked, ref)
	}
	rel, err := filepath.Rel(strings.TrimSpace(string(top)), abs)
	if err != nil {
		return nil, err
	}
	content, err := git(dir, "show", ref+":"+filepath.ToSlash(rel))
	if err != nil {
		return nil, nil // the ref has no such file
	}
	return content, nil
}

func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Output()
}

// schemaVersionOf parses a schema file name "MAJOR.MINOR.json".
func schemaVersionOf(name string) (major, minor int, ok bool) {
	version, ok := strings.CutSuffix(name, ".json")
	if !ok {
		return 0, 0, false
	}
	majorText, minorText, ok := strings.Cut(version, ".")
	if !ok {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(majorText)
	minor, err2 := strconv.Atoi(minorText)
	return major, minor, err1 == nil && err2 == nil
}

// olderSchemas lists the schema files in the directory of current that are
// not current, by name.
func olderSchemas(current string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Dir(current))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if _, _, ok := schemaVersionOf(e.Name()); ok && e.Name() != filepath.Base(current) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func readFrozen(dir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, frozenFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	frozen := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		hash, name, ok := strings.Cut(scanner.Text(), "  ")
		if !ok {
			return nil, fmt.Errorf("%s: malformed line %q", frozenFile, scanner.Text())
		}
		frozen[name] = hash
	}
	return frozen, scanner.Err()
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// checkFrozen reports every older schema of current's contract that is not
// frozen or no longer matches its frozen hash.
func checkFrozen(current string) []string {
	dir := filepath.Dir(current)
	frozen, err := readFrozen(dir)
	if err != nil {
		return []string{err.Error()}
	}
	older, err := olderSchemas(current)
	if err != nil {
		return []string{err.Error()}
	}
	var problems []string
	for _, name := range older {
		path := filepath.Join(dir, name)
		hash, ok := frozen[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is an older version but not frozen: run make schemas", path))
			continue
		}
		got, err := fileHash(path)
		if err != nil {
			problems = append(problems, err.Error())
		} else if got != hash {
			problems = append(problems, fmt.Sprintf("%s is a published older version and must not change (ADR 0005); restore it", path))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(frozen)) {
		if !slices.Contains(older, name) {
			problems = append(problems, fmt.Sprintf("%s lists %s, which is missing or is the current version", filepath.Join(dir, frozenFile), name))
		}
	}
	return problems
}

// freezeOlder adds every older schema of current's contract that is not yet
// frozen to frozenFile. It never rewrites a recorded hash.
func freezeOlder(current string) error {
	dir := filepath.Dir(current)
	frozen, err := readFrozen(dir)
	if err != nil {
		return err
	}
	older, err := olderSchemas(current)
	if err != nil {
		return err
	}
	added := false
	for _, name := range older {
		if _, ok := frozen[name]; ok {
			continue
		}
		if frozen[name], err = fileHash(filepath.Join(dir, name)); err != nil {
			return err
		}
		added = true
	}
	if !added {
		return nil
	}
	var buf bytes.Buffer
	for _, name := range slices.Sorted(maps.Keys(frozen)) {
		fmt.Fprintf(&buf, "%s  %s\n", frozen[name], name)
	}
	return os.WriteFile(filepath.Join(dir, frozenFile), buf.Bytes(), 0o644)
}

// previousMinor returns the path of the newest schema of current's MAJOR
// older than current, or "" if current is the first MINOR.
func previousMinor(current string) (string, error) {
	major, minor, ok := schemaVersionOf(filepath.Base(current))
	if !ok {
		return "", fmt.Errorf("%s: not a MAJOR.MINOR.json file", current)
	}
	older, err := olderSchemas(current)
	if err != nil {
		return "", err
	}
	best, bestMinor := "", -1
	for _, name := range older {
		m, n, _ := schemaVersionOf(name)
		if m == major && n < minor && n > bestMinor {
			best, bestMinor = name, n
		}
	}
	if best == "" {
		return "", nil
	}
	return filepath.Join(filepath.Dir(current), best), nil
}

// incompatibleWithPrevious lists why the schema generated for current is not
// a compatible MINOR of the previous one, or nil (also when there is none).
func incompatibleWithPrevious(current string, generated []byte) ([]string, error) {
	prevPath, err := previousMinor(current)
	if err != nil || prevPath == "" {
		return nil, err
	}
	data, err := os.ReadFile(prevPath)
	if err != nil {
		return nil, err
	}
	var older, newer map[string]any
	if err := json.Unmarshal(data, &older); err != nil {
		return nil, fmt.Errorf("%s: %w", prevPath, err)
	}
	if err := json.Unmarshal(generated, &newer); err != nil {
		return nil, err
	}
	return MinorIncompatibilities(older, newer), nil
}
