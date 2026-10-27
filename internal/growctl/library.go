package growctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// LibraryStep is one planned action on one library: ActionCreate or
// ActionUnchanged (apply), ActionDelete or ActionNotFound (delete -f).
type LibraryStep struct {
	Action Action
	Name   string
	Server apiv0.LibraryResponse
}

// WidgetTypeStep is one planned action on one WidgetType of a library's
// Draft.
type WidgetTypeStep struct {
	Action  Action
	Library string
	// Manifest is set for every action but ActionPrune and ActionDelete.
	Manifest WidgetTypeManifest
	// Server is set for every action but ActionCreate and ActionNotFound.
	Server apiv0.WidgetTypeResponse
}

// Name returns the name of the WidgetType the step acts on.
func (s WidgetTypeStep) Name() string {
	if s.Manifest.Name() != "" {
		return s.Manifest.Name()
	}
	return s.Server.Name
}

// IsChange reports whether executing the step modifies the server.
func (s WidgetTypeStep) IsChange() bool { return s.Action != ActionUnchanged && s.Action != ActionNotFound }

// PinStep is one planned Library Pin of a project: ActionCreate adds it,
// ActionUpdate switches it from release From, ActionUnchanged leaves it.
type PinStep struct {
	Action  Action
	Project string
	Library string
	Release int
	From    int
	Version int
}

// libraryPlan is what apply does to the libraries, the WidgetTypes of their
// Drafts and the projects' pins, after the projects and before the tags.
type libraryPlan struct {
	libraries   []LibraryStep
	widgetTypes []WidgetTypeStep
	pins        []PinStep
}

func (p libraryPlan) changed() bool {
	for _, s := range p.libraries {
		if s.Action == ActionCreate {
			return true
		}
	}
	for _, s := range p.widgetTypes {
		if s.IsChange() {
			return true
		}
	}
	for _, s := range p.pins {
		if s.Action != ActionUnchanged {
			return true
		}
	}
	return false
}

// sameDefinition reports whether the WidgetType on the server is defined as
// its manifest declares it.
func sameDefinition(server apiv0.WidgetTypeResponse, m apiv0.CreateWidgetTypeRequest) bool {
	normalize := func(ports []apiv0.InputPort) []apiv0.InputPort {
		if len(ports) == 0 {
			return nil
		}
		return ports
	}
	return server.Name == m.Name && server.HtmlTemplate == m.HtmlTemplate && server.Script == m.Script &&
		server.ScriptLanguage == m.ScriptLanguage && server.DefaultWidth == m.DefaultWidth &&
		server.DefaultHeight == m.DefaultHeight && slices.Equal(normalize(server.InputPorts), normalize(m.InputPorts))
}

// planLibraries plans the manifests' libraries, WidgetTypes and pins
// against the server: missing libraries and WidgetTypes are created, the
// WidgetTypes that differ updated and, with prune, those absent from the
// manifests removed from every library the manifests declare WidgetTypes
// of; the pins a project's spec declares are added or switched. A
// WidgetType of a library the plan creates is created with it. Manifests
// without libraries, WidgetTypes and pins plan nothing and read nothing.
func planLibraries(ctx context.Context, client *Client, manifests Manifests, prune bool) (libraryPlan, error) {
	var plan libraryPlan
	if !declaresLibraries(manifests) {
		return plan, nil
	}
	libraries, err := client.ListLibraries(ctx)
	if err != nil {
		return plan, err
	}
	byName := make(map[string]apiv0.LibraryResponse, len(libraries))
	for _, l := range libraries {
		byName[l.Name] = l
	}
	created := map[string]bool{}
	for _, m := range manifests.Libraries {
		if existing, ok := byName[m.Name]; ok {
			plan.libraries = append(plan.libraries, LibraryStep{Action: ActionUnchanged, Name: m.Name, Server: existing})
			continue
		}
		plan.libraries = append(plan.libraries, LibraryStep{Action: ActionCreate, Name: m.Name})
		created[m.Name] = true
	}

	// The WidgetTypes, library by library in the order first declared.
	var order []string
	declared := map[string][]WidgetTypeManifest{}
	for _, m := range manifests.WidgetTypes {
		if _, ok := declared[m.Library]; !ok {
			order = append(order, m.Library)
		}
		declared[m.Library] = append(declared[m.Library], m)
	}
	for _, libraryName := range order {
		var server []apiv0.WidgetTypeResponse
		if lib, ok := byName[libraryName]; ok {
			if server, err = client.Library(lib.ID).ListWidgetTypes(ctx); err != nil {
				return plan, fmt.Errorf("library/%s: %w", libraryName, err)
			}
		} else if !created[libraryName] {
			return plan, fmt.Errorf("library/%s not found: declare it with kind: Library or create it first", libraryName)
		}
		serverByName := make(map[string]apiv0.WidgetTypeResponse, len(server))
		for _, wt := range server {
			serverByName[wt.Name] = wt
		}
		names := map[string]bool{}
		for _, m := range declared[libraryName] {
			names[m.Name()] = true
			existing, ok := serverByName[m.Name()]
			switch {
			case !ok:
				plan.widgetTypes = append(plan.widgetTypes, WidgetTypeStep{Action: ActionCreate, Library: libraryName, Manifest: m})
			case sameDefinition(existing, m.Definition):
				plan.widgetTypes = append(plan.widgetTypes, WidgetTypeStep{Action: ActionUnchanged, Library: libraryName, Manifest: m, Server: existing})
			default:
				plan.widgetTypes = append(plan.widgetTypes, WidgetTypeStep{Action: ActionUpdate, Library: libraryName, Manifest: m, Server: existing})
			}
		}
		if prune {
			var pruned []WidgetTypeStep
			for _, wt := range server {
				if !names[wt.Name] {
					pruned = append(pruned, WidgetTypeStep{Action: ActionPrune, Library: libraryName, Server: wt})
				}
			}
			sort.Slice(pruned, func(i, j int) bool { return pruned[i].Server.Name < pruned[j].Server.Name })
			plan.widgetTypes = append(plan.widgetTypes, pruned...)
		}
	}

	// The pins of the projects whose spec declares them.
	for _, m := range manifests.Projects {
		if len(m.LibraryPins) == 0 {
			continue
		}
		pinned := map[uuid.UUID]apiv0.LibraryPinResponse{}
		if found, ok, err := client.FindProjectByName(ctx, m.Name); err != nil {
			return plan, err
		} else if ok {
			pins, err := client.Project(found.ID).ListLibraryPins(ctx)
			if err != nil {
				return plan, fmt.Errorf("project/%s: %w", m.Name, err)
			}
			for _, p := range pins {
				pinned[p.LibraryID] = p
			}
		}
		libraryNames := make([]string, 0, len(m.LibraryPins))
		for name := range m.LibraryPins {
			libraryNames = append(libraryNames, name)
		}
		sort.Strings(libraryNames)
		for _, libraryName := range libraryNames {
			release := m.LibraryPins[libraryName]
			step := PinStep{Action: ActionCreate, Project: m.Name, Library: libraryName, Release: release}
			if lib, ok := byName[libraryName]; ok {
				if p, ok := pinned[lib.ID]; ok {
					step.Version, step.From = p.Version, p.Release
					step.Action = ActionUpdate
					if p.Release == release {
						step.Action = ActionUnchanged
					}
				}
			} else if !created[libraryName] {
				return plan, fmt.Errorf("project/%s: library/%s not found", m.Name, libraryName)
			}
			plan.pins = append(plan.pins, step)
		}
	}
	return plan, nil
}

// printLibraryDiff prints what the plan would change, for diff.
func printLibraryDiff(out io.Writer, plan libraryPlan) {
	for _, s := range plan.libraries {
		if s.Action == ActionCreate {
			fmt.Fprintf(out, "+ library/%s\n", s.Name)
		}
	}
	for _, s := range plan.widgetTypes {
		switch s.Action {
		case ActionCreate:
			fmt.Fprintf(out, "+ %s\n", widgetTypeRef(s.Library, s.Name()))
		case ActionUpdate:
			fmt.Fprintf(out, "~ %s\n", widgetTypeRef(s.Library, s.Name()))
		case ActionPrune:
			fmt.Fprintf(out, "- %s\n", widgetTypeRef(s.Library, s.Name()))
		}
	}
	for _, s := range plan.pins {
		switch s.Action {
		case ActionCreate:
			fmt.Fprintf(out, "+ project/%s pin library/%s release %d\n", s.Project, s.Library, s.Release)
		case ActionUpdate:
			fmt.Fprintf(out, "~ project/%s pin library/%s release %d -> %d\n", s.Project, s.Library, s.From, s.Release)
		}
	}
}

// executeLibraries runs the plan's libraries, WidgetTypes and pins, printing
// each step once it is done. It stops at the first failure.
func executeLibraries(ctx context.Context, client *Client, plan libraryPlan, out io.Writer) error {
	ids := map[string]apiv0.LibraryResponse{}
	for _, s := range plan.libraries {
		switch s.Action {
		case ActionCreate:
			resp, err := client.CreateLibrary(ctx, apiv0.CreateLibraryRequest{Name: s.Name})
			if errors.Is(err, apiclient.ErrConflict) {
				s.Action = ActionUnchanged // created meanwhile: the desired state is reached
			} else if err != nil {
				return fmt.Errorf("library/%s: %w", s.Name, err)
			} else {
				ids[s.Name] = apiv0.LibraryResponse{ID: resp.ID, Name: s.Name, Version: resp.Version}
			}
		default:
			ids[s.Name] = s.Server
		}
		fmt.Fprintf(out, "library/%s %s\n", s.Name, actionResult(s.Action))
	}
	libraryOf := func(name string) (*apiclient.LibraryClient, error) {
		if lib, ok := ids[name]; ok {
			return client.Library(lib.ID), nil
		}
		found, ok, err := client.FindLibraryByName(ctx, name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("library/%s not found", name)
		}
		ids[name] = found
		return client.Library(found.ID), nil
	}

	for _, s := range plan.widgetTypes {
		ref := widgetTypeRef(s.Library, s.Name())
		lib, err := libraryOf(s.Library)
		if err != nil {
			return fmt.Errorf("%s: %w", ref, err)
		}
		switch s.Action {
		case ActionCreate:
			_, err = lib.CreateWidgetType(ctx, s.Manifest.Definition)
		case ActionUpdate:
			d := s.Manifest.Definition
			_, err = lib.UpdateWidgetType(ctx, s.Server.ID, apiv0.UpdateWidgetTypeRequest{
				Name: d.Name, HtmlTemplate: d.HtmlTemplate, Script: d.Script, ScriptLanguage: d.ScriptLanguage,
				DefaultWidth: d.DefaultWidth, DefaultHeight: d.DefaultHeight, InputPorts: d.InputPorts, Version: s.Server.Version,
			})
		case ActionPrune, ActionDelete:
			if _, err = lib.DeleteWidgetType(ctx, s.Server.ID); errors.Is(err, apiclient.ErrNotFound) {
				err = nil // already gone — the desired state is reached
			}
		}
		if err != nil {
			return fmt.Errorf("%s: %w", ref, err)
		}
		fmt.Fprintf(out, "%s %s\n", ref, actionResult(s.Action))
	}

	for _, s := range plan.pins {
		if s.Action != ActionUnchanged {
			lib, err := libraryOf(s.Library)
			if err != nil {
				return fmt.Errorf("project/%s: %w", s.Project, err)
			}
			found, ok, err := client.FindProjectByName(ctx, s.Project)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("project/%s not found", s.Project)
			}
			_, err = client.Project(found.ID).PinLibrary(ctx, lib.ID(), apiv0.PutLibraryPinRequest{Release: s.Release, Version: s.Version})
			if err != nil {
				return fmt.Errorf("project/%s pin library/%s release %d: %w%s", s.Project, s.Library, s.Release, err, orphanedHint(err))
			}
		}
		fmt.Fprintf(out, "project/%s pin library/%s release %d %s\n", s.Project, s.Library, s.Release, pinResult(s.Action))
	}
	return nil
}

// orphanedHint names the Widgets a refused switch of a pin would orphan.
func orphanedHint(err error) string {
	var se *apiclient.StatusError
	if !errors.As(err, &se) || len(se.Problem.OrphanedWidgets) == 0 {
		return ""
	}
	hint := "\n  widgets left without their widget type:"
	for _, w := range se.Problem.OrphanedWidgets {
		hint += fmt.Sprintf("\n    %s (scene %s)", w.Name, w.SceneID)
	}
	return hint
}

func pinResult(a Action) string {
	switch a {
	case ActionCreate:
		return "added"
	case ActionUpdate:
		return "switched"
	}
	return actionResult(a)
}

// declaresLibraries reports whether the manifests declare a library, a
// WidgetType or a project's pins.
func declaresLibraries(manifests Manifests) bool {
	if len(manifests.Libraries) > 0 || len(manifests.WidgetTypes) > 0 {
		return true
	}
	return slices.ContainsFunc(manifests.Projects, func(m ProjectManifest) bool { return len(m.LibraryPins) > 0 })
}

// planDeleteLibraries plans delete -f of the manifests' WidgetTypes, by
// name in their library, and of their libraries.
func planDeleteLibraries(ctx context.Context, client *Client, manifests Manifests) ([]WidgetTypeStep, []LibraryStep, error) {
	if len(manifests.WidgetTypes) == 0 && len(manifests.Libraries) == 0 {
		return nil, nil, nil
	}
	libraries, err := client.ListLibraries(ctx)
	if err != nil {
		return nil, nil, err
	}
	byName := make(map[string]apiv0.LibraryResponse, len(libraries))
	for _, l := range libraries {
		byName[l.Name] = l
	}
	var types []WidgetTypeStep
	drafts := map[string]map[string]apiv0.WidgetTypeResponse{}
	for _, m := range manifests.WidgetTypes {
		lib, ok := byName[m.Library]
		if !ok {
			types = append(types, WidgetTypeStep{Action: ActionNotFound, Library: m.Library, Manifest: m})
			continue
		}
		if _, read := drafts[m.Library]; !read {
			list, err := client.Library(lib.ID).ListWidgetTypes(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("library/%s: %w", m.Library, err)
			}
			drafts[m.Library] = map[string]apiv0.WidgetTypeResponse{}
			for _, wt := range list {
				drafts[m.Library][wt.Name] = wt
			}
		}
		if wt, ok := drafts[m.Library][m.Name()]; ok {
			types = append(types, WidgetTypeStep{Action: ActionDelete, Library: m.Library, Manifest: m, Server: wt})
		} else {
			types = append(types, WidgetTypeStep{Action: ActionNotFound, Library: m.Library, Manifest: m})
		}
	}
	var libs []LibraryStep
	for _, m := range manifests.Libraries {
		if lib, ok := byName[m.Name]; ok {
			libs = append(libs, LibraryStep{Action: ActionDelete, Name: m.Name, Server: lib})
		} else {
			libs = append(libs, LibraryStep{Action: ActionNotFound, Name: m.Name})
		}
	}
	return types, libs, nil
}

// executeDeleteLibraries deletes the WidgetTypes, then the libraries, of a
// delete -f, printing each step once it is done.
func executeDeleteLibraries(ctx context.Context, client *Client, types []WidgetTypeStep, libs []LibraryStep, out io.Writer) error {
	for _, s := range types {
		ref := widgetTypeRef(s.Library, s.Name())
		if s.Action == ActionDelete {
			lib, ok, err := client.FindLibraryByName(ctx, s.Library)
			if err != nil {
				return fmt.Errorf("%s: %w", ref, err)
			}
			if ok {
				_, err = client.Library(lib.ID).DeleteWidgetType(ctx, s.Server.ID)
			}
			if !ok || errors.Is(err, apiclient.ErrNotFound) {
				s.Action, err = ActionNotFound, nil
			}
			if err != nil {
				return fmt.Errorf("%s: %w", ref, err)
			}
		}
		fmt.Fprintf(out, "%s %s\n", ref, actionResult(s.Action))
	}
	for _, s := range libs {
		if s.Action == ActionDelete {
			_, err := client.DeleteLibrary(ctx, s.Server.ID)
			if errors.Is(err, apiclient.ErrNotFound) {
				s.Action, err = ActionNotFound, nil
			}
			if err != nil {
				return fmt.Errorf("library/%s: %w", s.Name, err)
			}
		}
		fmt.Fprintf(out, "library/%s %s\n", s.Name, actionResult(s.Action))
	}
	return nil
}

// findLibrary returns the client of the library with the given name.
func findLibrary(ctx context.Context, client *Client, name string) (*apiclient.LibraryClient, apiv0.LibraryResponse, error) {
	found, ok, err := client.FindLibraryByName(ctx, name)
	if err != nil {
		return nil, found, err
	}
	if !ok {
		return nil, found, fmt.Errorf("library/%s not found", name)
	}
	return client.Library(found.ID), found, nil
}

func newReleaseCommand(s *settings) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "release <library> [--note <note>]",
		Short: "Release the Draft of a library as its next Library Release",
		Long: "Make the next Library Release of the library from its Draft as it is now. A\n" +
			"Draft unchanged since the latest release, or one being discarded, is not\n" +
			"released. Projects use the release once they pin it (spec.libraryPins).",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			lib, found, err := findLibrary(ctx, s.client(), args[0])
			if err != nil {
				return err
			}
			release, err := lib.Release(ctx, apiv0.CreateReleaseRequest{Note: note, Version: found.Version})
			if err != nil {
				return fmt.Errorf("library/%s: %w", args[0], err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "library/%s released %d\n", args[0], release.Number)
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "the release's note")
	return cmd
}

func newDiscardCommand(s *settings) *cobra.Command {
	return &cobra.Command{
		Use:   "discard <library>",
		Short: "Reset the Draft of a library to its latest Library Release",
		Long: "Reset the Draft of the library to its latest release: the WidgetTypes added\n" +
			"since are removed, the changed ones restored, the removed ones brought back.\n" +
			"The server does it after the command returns; until then the Draft is\n" +
			"read-only.",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			lib, _, err := findLibrary(ctx, s.client(), args[0])
			if err != nil {
				return err
			}
			marked, err := lib.Discard(ctx)
			if err != nil {
				return fmt.Errorf("library/%s: %w", args[0], err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "library/%s discarding to release %d\n", args[0], marked.DiscardingTo)
			return nil
		},
	}
}

// exactlyOneArg requires one positional argument, a usage error otherwise
// (exit status 2).
func exactlyOneArg(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return usageError(err)
	}
	return nil
}

func newGetLibrariesCommand(s *settings) *cobra.Command {
	return &cobra.Command{
		Use:     "libraries",
		Aliases: []string{"library"},
		Short:   "List libraries",
		Long:    "List the server's libraries in creation order, with their latest release.",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			libraries, err := s.client().ListLibraries(cmd.Context())
			if err != nil {
				return err
			}
			if len(libraries) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "No libraries found.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tRELEASE\tUNRELEASED\tDISCARDING")
			for _, l := range libraries {
				discarding := ""
				if l.DiscardingTo != 0 {
					discarding = fmt.Sprint(l.DiscardingTo)
				}
				fmt.Fprintf(w, "%s\t%d\t%t\t%s\n", l.Name, l.LatestRelease, l.HasUnreleasedChanges, discarding)
			}
			return w.Flush()
		},
	}
}
