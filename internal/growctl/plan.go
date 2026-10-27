package growctl

import (
	"fmt"
	"sort"
	"strings"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// ServerTag is a tag that exists on the server. The planner uses ID, Name and
// Type; get prints the rest.
type ServerTag = apiv0.TagResponse

// Action is what a Step does to one tag.
type Action int

const (
	// ActionCreate creates a tag declared in the manifest but missing on the server.
	ActionCreate Action = iota + 1
	// ActionUnchanged leaves a tag that already matches the manifest as is.
	ActionUnchanged
	// ActionPrune deletes a server tag that is absent from the manifest (apply --prune).
	ActionPrune
	// ActionDelete deletes a server tag declared in the manifest (delete -f).
	ActionDelete
	// ActionNotFound reports a tag declared in the manifest but missing on the server (delete -f).
	ActionNotFound
	// ActionUpdate changes a WidgetType of a Library's Draft to its
	// manifest, or switches a Library Pin to the release the manifest names.
	ActionUpdate
)

// Step is one planned action on one tag.
type Step struct {
	Action Action
	// Manifest is set for ActionCreate and ActionUnchanged.
	Manifest TagManifest
	// Server is set for ActionUnchanged, ActionPrune and ActionDelete.
	Server ServerTag
}

// Name returns the name of the tag the step acts on.
func (s Step) Name() string {
	if s.Manifest.Name != "" {
		return s.Manifest.Name
	}
	return s.Server.Name
}

// IsChange reports whether executing the step modifies the server.
func (s Step) IsChange() bool {
	return s.Action == ActionCreate || s.Action == ActionPrune || s.Action == ActionDelete
}

// PlanApply computes the steps that bring the server to the manifests: missing
// tags are created, and with prune every server tag absent from the manifests is
// deleted. A tag whose type differs from the manifest cannot be changed in place,
// so any such mismatch fails the whole plan before anything is changed.
// Steps follow manifest order; prunes come last, sorted by name.
func PlanApply(manifests []TagManifest, server []ServerTag, prune bool) ([]Step, error) {
	byName := indexByName(server)

	var mismatches []string
	steps := make([]Step, 0, len(manifests))
	for _, m := range manifests {
		existing, ok := byName[m.Name]
		switch {
		case !ok:
			steps = append(steps, Step{Action: ActionCreate, Manifest: m})
		case existing.Type != m.Type:
			mismatches = append(mismatches,
				fmt.Sprintf("tag/%s: type is %s on the server, %s in the manifest", m.Name, existing.Type, m.Type))
		default:
			steps = append(steps, Step{Action: ActionUnchanged, Manifest: m, Server: existing})
		}
	}
	if len(mismatches) > 0 {
		return nil, fmt.Errorf("cannot change the type of an existing tag (delete it first):\n  %s",
			strings.Join(mismatches, "\n  "))
	}

	if prune {
		declared := make(map[string]bool, len(manifests))
		for _, m := range manifests {
			declared[m.Name] = true
		}
		var pruned []Step
		for _, s := range server {
			if !declared[s.Name] {
				pruned = append(pruned, Step{Action: ActionPrune, Server: s})
			}
		}
		sort.Slice(pruned, func(i, j int) bool { return pruned[i].Server.Name < pruned[j].Server.Name })
		steps = append(steps, pruned...)
	}
	return steps, nil
}

// PlanDelete computes the steps that delete the manifests' tags by name.
// The spec is ignored; a tag missing on the server is reported, not an error.
func PlanDelete(manifests []TagManifest, server []ServerTag) []Step {
	byName := indexByName(server)
	steps := make([]Step, 0, len(manifests))
	for _, m := range manifests {
		if existing, ok := byName[m.Name]; ok {
			steps = append(steps, Step{Action: ActionDelete, Server: existing})
		} else {
			steps = append(steps, Step{Action: ActionNotFound, Manifest: m})
		}
	}
	return steps
}

func indexByName(server []ServerTag) map[string]ServerTag {
	byName := make(map[string]ServerTag, len(server))
	for _, s := range server {
		byName[s.Name] = s
	}
	return byName
}

// ServerProject is a project that exists on the server.
type ServerProject = apiv0.ProjectResponse

// ProjectStep is one planned action on one project: ActionCreate or
// ActionUnchanged (apply), ActionDelete or ActionNotFound (delete -f).
// Projects are never pruned.
type ProjectStep struct {
	Action Action
	Name   string
	// Server is set for ActionUnchanged and ActionDelete.
	Server ServerProject
}

// IsChange reports whether executing the step modifies the server.
func (s ProjectStep) IsChange() bool {
	return s.Action == ActionCreate || s.Action == ActionDelete
}

// PlanProjects computes the steps of the manifests' projects, in manifest
// order, given those of them that exist on the server: apply creates the
// missing ones (forDelete false), delete -f deletes the existing ones.
func PlanProjects(manifests []ProjectManifest, server []ServerProject, forDelete bool) []ProjectStep {
	byName := make(map[string]ServerProject, len(server))
	for _, p := range server {
		byName[p.Name] = p
	}
	steps := make([]ProjectStep, 0, len(manifests))
	for _, m := range manifests {
		existing, ok := byName[m.Name]
		switch {
		case ok && forDelete:
			steps = append(steps, ProjectStep{Action: ActionDelete, Name: m.Name, Server: existing})
		case ok:
			steps = append(steps, ProjectStep{Action: ActionUnchanged, Name: m.Name, Server: existing})
		case forDelete:
			steps = append(steps, ProjectStep{Action: ActionNotFound, Name: m.Name})
		default:
			steps = append(steps, ProjectStep{Action: ActionCreate, Name: m.Name})
		}
	}
	return steps
}
