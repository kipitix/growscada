// Package growctl implements growctl — a kubectl-style CLI that declaratively
// manages GrowSCADA entities (Projects and the Tags of a Project) from YAML
// manifests through the REST API.
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

	"github.com/kipitix/growscada/contract"
	manifestv0 "github.com/kipitix/growscada/contract/manifest/v0"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// errNoManifest is returned when no manifest source is given.
var errNoManifest = errors.New("no manifest given: use -f <file|directory|->")

// TagManifest is a validated Tag document from a manifest: the Tag's
// definition. Project is metadata.project, "" when the document names none.
type TagManifest struct {
	Name    string
	Project string
	Type    string
}

// ProjectManifest is a validated Project document from a manifest.
type ProjectManifest struct {
	Name string
}

// Manifests are the documents of one or more manifests, by kind, each in
// the order declared.
type Manifests struct {
	Projects []ProjectManifest
	Tags     []TagManifest
}

func (m *Manifests) add(other Manifests) {
	m.Projects = append(m.Projects, other.Projects...)
	m.Tags = append(m.Tags, other.Tags...)
}

// ReadManifests reads manifests from the given sources: a file, a directory
// (its *.yaml and *.yml files, non-recursive, in name order) or "-" for stdin.
// Project names and Tag names must each be unique across all sources.
func ReadManifests(sources []string, stdin io.Reader) (Manifests, error) {
	if len(sources) == 0 {
		return Manifests{}, errNoManifest
	}

	var all Manifests
	for _, source := range sources {
		files, err := expandSource(source)
		if err != nil {
			return Manifests{}, err
		}
		for _, file := range files {
			var data []byte
			if file == "-" {
				data, err = io.ReadAll(stdin)
			} else {
				data, err = os.ReadFile(file)
			}
			if err != nil {
				return Manifests{}, fmt.Errorf("cannot read %s: %w", file, err)
			}
			manifests, err := ParseManifests(data)
			if err != nil {
				return Manifests{}, fmt.Errorf("%s: %w", file, err)
			}
			all.add(manifests)
		}
	}

	seenProjects := make(map[string]bool, len(all.Projects))
	for _, m := range all.Projects {
		if seenProjects[m.Name] {
			return Manifests{}, fmt.Errorf("project/%s is declared more than once", m.Name)
		}
		seenProjects[m.Name] = true
	}
	seenTags := make(map[string]bool, len(all.Tags))
	for _, m := range all.Tags {
		if seenTags[m.Name] {
			return Manifests{}, fmt.Errorf("tag/%s is declared more than once", m.Name)
		}
		seenTags[m.Name] = true
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
func ParseManifests(data []byte) (Manifests, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var manifests Manifests
	for docIndex := 1; ; docIndex++ {
		var node yaml.Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			return manifests, nil
		}
		if err != nil {
			return Manifests{}, fmt.Errorf("document %d: %w", docIndex, err)
		}
		if isEmptyDocument(&node) {
			continue
		}
		if err := parseDocument(&node, &manifests); err != nil {
			return Manifests{}, fmt.Errorf("document %d: %w", docIndex, err)
		}
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

// parseDocument validates one document and adds it to manifests by its kind.
func parseDocument(node *yaml.Node, manifests *Manifests) error {
	var header manifestv0.Header
	if err := node.Decode(&header); err != nil {
		return err
	}
	version, err := checkAPIVersion(header.APIVersion)
	if err != nil {
		return err
	}
	switch header.Kind {
	case manifestv0.KindTag:
		if err := checkRemovedTagFields(node); err != nil {
			return err
		}
		var doc manifestv0.TagDocument
		if err := decodeStrict(node, &doc); err != nil {
			return err
		}
		m, err := validateTag(doc)
		if err != nil {
			return err
		}
		if m.Project != "" {
			if err := requireAPIVersion(version, projectsSince, "tag/"+m.Name+": metadata.project"); err != nil {
				return err
			}
		}
		manifests.Tags = append(manifests.Tags, m)
	case manifestv0.KindProject:
		if err := requireAPIVersion(version, projectsSince, "kind "+manifestv0.KindProject); err != nil {
			return err
		}
		var doc manifestv0.ProjectDocument
		if err := decodeStrict(node, &doc); err != nil {
			return err
		}
		m, err := validateProject(doc)
		if err != nil {
			return err
		}
		manifests.Projects = append(manifests.Projects, m)
	default:
		return fmt.Errorf("unsupported kind %q, expected %q or %q", header.Kind, manifestv0.KindProject, manifestv0.KindTag)
	}
	return nil
}

// removedTagFields are the spec fields of a Tag that apiVersion 0.3 removed:
// a Tag's value and quality are its state, not configuration (ADR 0011).
var removedTagFields = []string{"initialValue", "initialQuality"}

// checkRemovedTagFields names a removed spec field of a Tag document rather
// than leaving it to the strict decode's "unknown field", whatever the
// document's apiVersion: no version of growctl applies them now.
func checkRemovedTagFields(node *yaml.Node) error {
	var doc struct {
		Metadata struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec map[string]yaml.Node `yaml:"spec"`
	}
	if err := node.Decode(&doc); err != nil {
		return nil // the strict decode reports it
	}
	for _, field := range removedTagFields {
		if _, ok := doc.Spec[field]; ok {
			return fmt.Errorf("tag/%s: spec.%s was removed in apiVersion %q: a tag's state starts as its type's zero value "+
				"with quality bad until a supplier writes one; remove the field", doc.Metadata.Name, field, manifestv0.FormatAPIVersion(tagStateSince))
		}
	}
	return nil
}

// tagStateSince is the manifest version that removed a Tag's initial state.
var tagStateSince = contract.SchemaVersion{Major: 0, Minor: 3}

// decodeStrict decodes a document rejecting unknown fields: yaml.Node.Decode
// cannot, so the document is re-encoded and decoded strictly.
func decodeStrict(node *yaml.Node, doc any) error {
	raw, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	strict := yaml.NewDecoder(bytes.NewReader(raw))
	strict.KnownFields(true)
	return strict.Decode(doc)
}

func validateProject(doc manifestv0.ProjectDocument) (ProjectManifest, error) {
	name := doc.Metadata.Name
	if name == "" {
		return ProjectManifest{}, errors.New("metadata.name is required")
	}
	if _, err := project.NewProjectName(name); err != nil {
		return ProjectManifest{}, fmt.Errorf("project/%s: invalid metadata.name: %w", name, err)
	}
	return ProjectManifest{Name: name}, nil
}

// preVersioningAPIVersion is what manifests carried before ADR 0005: the
// format of today's v0. growctl names the fix rather than calling it an
// unknown version. Remove it when the manifests reach 1.0, where the label
// takes its real meaning.
const preVersioningAPIVersion = "growscada/v1"

// checkAPIVersion accepts any MINOR up to the one growctl knows of its
// MAJOR and returns it. The check runs before the strict decode, so a field
// from a newer MINOR is reported as "update growctl" rather than as an
// unknown field.
func checkAPIVersion(apiVersion string) (contract.SchemaVersion, error) {
	if apiVersion == preVersioningAPIVersion {
		return contract.SchemaVersion{}, fmt.Errorf("apiVersion %q is the format from before contract versioning: replace it with %q",
			apiVersion, manifestv0.APIVersion())
	}
	v, err := manifestv0.ParseAPIVersion(apiVersion)
	if err != nil || !v.SameMajor(manifestv0.SchemaVersion()) {
		return contract.SchemaVersion{}, fmt.Errorf("unsupported apiVersion %q, expected %q", apiVersion, manifestv0.APIVersion())
	}
	if v.NewerThan(manifestv0.SchemaVersion()) {
		return contract.SchemaVersion{}, fmt.Errorf("apiVersion %q is newer than this growctl understands (%s): update growctl",
			apiVersion, manifestv0.APIVersion())
	}
	return v, nil
}

// projectsSince is the manifest version that brought kind Project and
// metadata.project.
var projectsSince = contract.SchemaVersion{Major: 0, Minor: 2}

// requireAPIVersion rejects a feature of a document whose apiVersion v is
// older than the one that brought it (since): a growctl of v would not
// understand the document (ADR 0005).
func requireAPIVersion(v, since contract.SchemaVersion, feature string) error {
	if since.NewerThan(v) {
		return fmt.Errorf("%s appeared in apiVersion %q, but the document states %q: raise its apiVersion",
			feature, manifestv0.FormatAPIVersion(since), manifestv0.FormatAPIVersion(v))
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
	if doc.Metadata.Project != "" {
		if _, err := project.NewProjectName(doc.Metadata.Project); err != nil {
			return TagManifest{}, fmt.Errorf("tag/%s: invalid metadata.project: %w", name, err)
		}
	}

	if doc.Spec.Type == nil {
		return TagManifest{}, fmt.Errorf("tag/%s: spec.type is required", name)
	}
	tagType, err := tag.NewTagType(*doc.Spec.Type)
	if err != nil {
		return TagManifest{}, fmt.Errorf("tag/%s: invalid spec.type: %w", name, err)
	}

	return TagManifest{Name: name, Project: doc.Metadata.Project, Type: tagType.String()}, nil
}
