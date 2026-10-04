// Package growctl implements growctl — a kubectl-style CLI that declaratively
// manages GrowSCADA entities (currently only Tags) from YAML manifests through
// the REST API.
package growctl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	manifestv0 "github.com/kipitix/growscada/contract/manifest/v0"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// errNoManifest is returned when no manifest source is given.
var errNoManifest = errors.New("no manifest given: use -f <file|directory|->")

// TagManifest is a validated Tag document from a manifest.
// InitialValue and InitialQuality are applied only when the tag is created.
type TagManifest struct {
	Name           string
	Type           string
	InitialValue   string
	InitialQuality string
}

// ReadManifests reads Tag manifests from the given sources: a file, a directory
// (its *.yaml and *.yml files, non-recursive, in name order) or "-" for stdin.
// Tag names must be unique across all sources.
func ReadManifests(sources []string, stdin io.Reader) ([]TagManifest, error) {
	if len(sources) == 0 {
		return nil, errNoManifest
	}

	var all []TagManifest
	for _, source := range sources {
		files, err := expandSource(source)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			var data []byte
			if file == "-" {
				data, err = io.ReadAll(stdin)
			} else {
				data, err = os.ReadFile(file)
			}
			if err != nil {
				return nil, fmt.Errorf("cannot read %s: %w", file, err)
			}
			manifests, err := ParseManifests(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", file, err)
			}
			all = append(all, manifests...)
		}
	}

	seen := make(map[string]bool, len(all))
	for _, m := range all {
		if seen[m.Name] {
			return nil, fmt.Errorf("tag/%s is declared more than once", m.Name)
		}
		seen[m.Name] = true
	}
	return all, nil
}

// expandSource turns a -f argument into the list of files to read.
func expandSource(source string) ([]string, error) {
	if source == "-" {
		return []string{"-"}, nil
	}
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("cannot open manifest: %w", err)
	}
	if !info.IsDir() {
		return []string{source}, nil
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return nil, fmt.Errorf("cannot read manifest directory: %w", err)
	}
	var files []string
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !e.IsDir() && (ext == ".yaml" || ext == ".yml") {
			files = append(files, filepath.Join(source, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// ParseManifests parses a multi-document YAML manifest. Empty documents are
// skipped; unknown fields, kinds and apiVersions are errors, and every spec
// field is required. A manifest of a newer MINOR than growctl knows is
// refused with a request to update growctl (ADR 0005).
func ParseManifests(data []byte) ([]TagManifest, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var manifests []TagManifest
	for docIndex := 1; ; docIndex++ {
		var node yaml.Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			return manifests, nil
		}
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", docIndex, err)
		}
		if isEmptyDocument(&node) {
			continue
		}
		m, err := parseDocument(&node)
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", docIndex, err)
		}
		manifests = append(manifests, m)
	}
}

// isEmptyDocument reports whether a document has no content (e.g. between two "---").
func isEmptyDocument(node *yaml.Node) bool {
	if len(node.Content) == 0 {
		return true
	}
	content := node.Content[0]
	return content.Kind == yaml.ScalarNode && content.Tag == "!!null"
}

func parseDocument(node *yaml.Node) (TagManifest, error) {
	var header manifestv0.Header
	if err := node.Decode(&header); err != nil {
		return TagManifest{}, err
	}
	if err := checkAPIVersion(header.APIVersion); err != nil {
		return TagManifest{}, err
	}
	if header.Kind != manifestv0.KindTag {
		return TagManifest{}, fmt.Errorf("unsupported kind %q, expected %q", header.Kind, manifestv0.KindTag)
	}

	// yaml.Node.Decode cannot reject unknown fields, so re-encode the document
	// and decode it strictly.
	raw, err := yaml.Marshal(node)
	if err != nil {
		return TagManifest{}, err
	}
	strict := yaml.NewDecoder(bytes.NewReader(raw))
	strict.KnownFields(true)
	var doc manifestv0.TagDocument
	if err := strict.Decode(&doc); err != nil {
		return TagManifest{}, err
	}
	return validateTag(doc)
}

// preVersioningAPIVersion is what manifests carried before ADR 0005: the
// format of today's v0. growctl names the fix rather than calling it an
// unknown version. Remove it when the manifests reach 1.0, where the label
// takes its real meaning.
const preVersioningAPIVersion = "growscada/v1"

// checkAPIVersion accepts any MINOR up to the one growctl knows of its
// MAJOR. The check runs before the strict decode, so a field from a newer
// MINOR is reported as "update growctl" rather than as an unknown field.
func checkAPIVersion(apiVersion string) error {
	if apiVersion == preVersioningAPIVersion {
		return fmt.Errorf("apiVersion %q is the format from before contract versioning: replace it with %q",
			apiVersion, manifestv0.APIVersion())
	}
	v, err := manifestv0.ParseAPIVersion(apiVersion)
	if err != nil || !v.SameMajor(manifestv0.SchemaVersion()) {
		return fmt.Errorf("unsupported apiVersion %q, expected %q", apiVersion, manifestv0.APIVersion())
	}
	if v.NewerThan(manifestv0.SchemaVersion()) {
		return fmt.Errorf("apiVersion %q is newer than this growctl understands (%s): update growctl",
			apiVersion, manifestv0.APIVersion())
	}
	return nil
}

func validateTag(doc manifestv0.TagDocument) (TagManifest, error) {
	name := doc.Metadata.Name
	if name == "" {
		return TagManifest{}, errors.New("metadata.name is required")
	}
	if _, err := tag.NewTagName(name); err != nil {
		return TagManifest{}, fmt.Errorf("tag/%s: invalid metadata.name: %w", name, err)
	}

	spec := doc.Spec
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"type", spec.Type},
		{"initialValue", spec.InitialValue},
		{"initialQuality", spec.InitialQuality},
	} {
		if field.value == nil {
			return TagManifest{}, fmt.Errorf("tag/%s: spec.%s is required", name, field.name)
		}
	}

	tagType, err := tag.NewTagType(*spec.Type)
	if err != nil {
		return TagManifest{}, fmt.Errorf("tag/%s: invalid spec.type: %w", name, err)
	}
	if _, err := tagType.NewTagValue(*spec.InitialValue); err != nil {
		return TagManifest{}, fmt.Errorf("tag/%s: invalid spec.initialValue: %w", name, err)
	}
	if _, err := tag.NewTagQuality(*spec.InitialQuality); err != nil {
		return TagManifest{}, fmt.Errorf("tag/%s: invalid spec.initialQuality: %w", name, err)
	}

	return TagManifest{
		Name:           name,
		Type:           tagType.String(),
		InitialValue:   *spec.InitialValue,
		InitialQuality: *spec.InitialQuality,
	}, nil
}
