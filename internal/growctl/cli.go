package growctl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

const (
	// ServerEnv is the environment variable holding the default server URL.
	ServerEnv = "GROWCTL_SERVER"
	// DefaultServer is used when neither --server nor GROWCTL_SERVER is set.
	DefaultServer = "http://localhost:9090"
)

// ExitError carries the process exit code for an error. A nil Err means the
// code is a result, not a failure (diff found changes), so nothing is printed.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// usageError marks an error as a usage error: exit status 2.
func usageError(err error) error {
	return &ExitError{Code: 2, Err: err}
}

// settings are the persistent flags every command reads.
type settings struct {
	server string
	// project is --project, "" when not given (see projectFor).
	project string
	errOut  io.Writer
}

// client returns a client of the server for one run.
func (s *settings) client() *Client {
	return NewClient(s.server, &http.Client{Timeout: 30 * time.Second}, s.errOut)
}

// NewRootCommand builds the growctl command tree. Output goes to the command's
// out writer (cmd.SetOut), manifests given as "-" are read from its input.
func NewRootCommand() *cobra.Command {
	s := &settings{server: os.Getenv(ServerEnv)}
	if s.server == "" {
		s.server = DefaultServer
	}

	root := &cobra.Command{
		Use:           "growctl",
		Short:         "Declaratively manage GrowSCADA projects and their tags from YAML manifests",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE:          runGroup,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			s.errOut = cmd.ErrOrStderr()
		},
	}
	// Usage errors exit with 2, so diff's exit status 1 always means "there are changes".
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError(err)
	})
	root.PersistentFlags().StringVar(&s.server, "server", s.server,
		"GrowSCADA REST API URL (env "+ServerEnv+")")
	root.PersistentFlags().StringVar(&s.project, "project", "",
		"project to work in (default: the context's, see growctl config use-project, or the manifests' metadata.project)")

	root.AddCommand(
		newApplyCommand(s),
		newDiffCommand(s),
		newDeleteCommand(s),
		newGetCommand(s),
		newConfigCommand(),
	)
	return root
}

// runGroup is the RunE of a command group: without arguments it prints help,
// an unknown subcommand is a usage error (exit status 2). Cobra itself reports
// it as a plain error, or — for a non-root group — prints help and succeeds.
// Groups set Args to cobra.ArbitraryArgs so that the argument reaches runGroup.
func runGroup(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	return usageError(fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath()))
}

// requireFiles reports a missing -f as a usage error (exit status 2).
func requireFiles(files []string) error {
	if len(files) == 0 {
		return usageError(errNoManifest)
	}
	return nil
}

// noArgs rejects positional arguments as a usage error (exit status 2).
func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return usageError(err)
	}
	return nil
}

func addFileFlag(cmd *cobra.Command, files *[]string) {
	cmd.Flags().StringArrayVarP(files, "filename", "f", nil,
		"manifest file, directory of *.yaml/*.yml files, or - for stdin (repeatable)")
}

// pruneFlags are apply/diff's --prune and the --pattern/--regex scope limiting it.
type pruneFlags struct {
	prune          bool
	pattern, regex string
}

func addPruneFlags(cmd *cobra.Command, f *pruneFlags, pruneHelp string) {
	cmd.Flags().BoolVar(&f.prune, "prune", false, pruneHelp)
	addNameFilterFlags(cmd, &f.pattern, &f.regex, "with --prune, only prune")
}

// scope returns the prune scope; --pattern/--regex without --prune is a usage error.
func (f pruneFlags) scope(cmd *cobra.Command) (NameFilter, error) {
	scope, err := nameFilterFromFlags(cmd, f.pattern, f.regex)
	if err != nil {
		return NameFilter{}, err
	}
	if scope.IsSet() && !f.prune {
		return NameFilter{}, usageError(fmt.Errorf("%s requires --prune", scope))
	}
	return scope, nil
}

func newApplyCommand(s *settings) *cobra.Command {
	var (
		files []string
		flags pruneFlags
	)
	cmd := &cobra.Command{
		Use:   "apply -f <file|directory|->",
		Short: "Create the manifest's projects and tags that are missing on the server",
		Long: "Create the manifest's projects and tags that are missing on the server; the\n" +
			"projects go first. initialValue and initialQuality are used only on creation;\n" +
			"existing tags keep their value. The tags are those of the project growctl works\n" +
			"in (--project). With --prune, the project's tags absent from the manifest are\n" +
			"deleted; --pattern or --regex limits pruning to the tags they select (the\n" +
			"manifest's own scope). Projects are never pruned.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFiles(files); err != nil {
				return err
			}
			scope, err := flags.scope(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			client := s.client()
			plan, err := planApply(ctx, client, s.project, files, cmd.InOrStdin(), flags.prune, scope)
			if err != nil {
				return err
			}
			return executeApply(ctx, client, plan, cmd.OutOrStdout())
		},
	}
	addFileFlag(cmd, &files)
	addPruneFlags(cmd, &flags, "delete the project's tags absent from the manifest")
	return cmd
}

func newDiffCommand(s *settings) *cobra.Command {
	var (
		files []string
		flags pruneFlags
	)
	cmd := &cobra.Command{
		Use:   "diff -f <file|directory|->",
		Short: "Show what apply would change, without changing anything",
		Long: "Show what apply would change, without changing anything.\n" +
			"Exit status: 0 — no changes, 1 — there are changes, 2 — error.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireFiles(files); err != nil {
				return err
			}
			scope, err := flags.scope(cmd)
			if err != nil {
				return err
			}
			plan, err := planApply(cmd.Context(), s.client(), s.project, files, cmd.InOrStdin(), flags.prune, scope)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			out := cmd.OutOrStdout()
			changed := false
			for _, step := range plan.projects {
				if step.Action == ActionCreate {
					fmt.Fprintf(out, "+ project/%s\n", step.Name)
				}
				changed = changed || step.IsChange()
			}
			for _, step := range plan.tags {
				switch step.Action {
				case ActionCreate:
					fmt.Fprintf(out, "+ tag/%s (%s)\n", step.Name(), step.Manifest.Type)
				case ActionPrune:
					fmt.Fprintf(out, "- tag/%s\n", step.Name())
				}
				changed = changed || step.IsChange()
			}
			if changed {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	addFileFlag(cmd, &files)
	addPruneFlags(cmd, &flags, "include the project's tags absent from the manifest")
	return cmd
}

func newDeleteCommand(s *settings) *cobra.Command {
	var (
		files          []string
		pattern, regex string
		yes            bool
	)
	cmd := &cobra.Command{
		Use:   "delete (-f <file|directory|-> | --pattern <pattern> | --regex <regex>)",
		Short: "Delete the manifest's projects and tags, or the tags matching a pattern",
		Long: "Delete by name: the tags declared in the manifest (-f; the spec is ignored,\n" +
			"missing ones are reported as not found), then its projects with all their\n" +
			"content — or the tags matching --pattern/--regex. The tags are those of the\n" +
			"project growctl works in (--project). Deleting by pattern lists the tags and\n" +
			"asks for confirmation unless --yes is given.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter, err := nameFilterFromFlags(cmd, pattern, regex)
			if err != nil {
				return err
			}
			switch {
			case filter.IsSet() && len(files) > 0:
				return usageError(errors.New("use either -f or --pattern/--regex, not both"))
			case filter.IsSet():
				return deleteMatching(cmd, s.client(), s.project, filter, yes)
			case len(files) == 0:
				return usageError(errors.New("nothing to delete: use -f <file|directory|-> or --pattern/--regex"))
			}
			return deleteManifests(cmd, s.client(), s.project, files)
		},
	}
	addFileFlag(cmd, &files)
	addNameFilterFlags(cmd, &pattern, &regex, "delete")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "delete by pattern without asking for confirmation")
	return cmd
}

// deleteManifests deletes the manifests' tags by name, in the project growctl
// works in, then their projects.
func deleteManifests(cmd *cobra.Command, client *Client, projectFlag string, files []string) error {
	ctx := cmd.Context()
	manifests, err := ReadManifests(files, cmd.InOrStdin())
	if err != nil {
		return err
	}
	var tags *Tags
	var tagSteps []Step
	if len(manifests.Tags) > 0 {
		name, err := projectFor(projectFlag, manifests.Tags)
		if err != nil {
			return err
		}
		var server []ServerTag
		found, ok, err := client.FindProjectByName(ctx, name)
		if err != nil {
			return err
		}
		if ok {
			tags = client.TagsOf(found)
			if server, err = fetchServerTags(ctx, tags, manifests.Tags, false, NameFilter{}); err != nil {
				return err
			}
		}
		tagSteps = PlanDelete(manifests.Tags, server)
	}
	existing, err := findProjects(ctx, client, manifests.Projects)
	if err != nil {
		return err
	}
	if err := executeDelete(ctx, tags, tagSteps, cmd.OutOrStdout()); err != nil {
		return err
	}
	return executeProjects(ctx, client, PlanProjects(manifests.Projects, existing, true), cmd.OutOrStdout())
}

// deleteMatching deletes the tags of the project growctl works in selected by
// the filter, after the user confirms the list (prompt on stderr, answer from
// stdin) unless yes.
func deleteMatching(cmd *cobra.Command, client *Client, projectFlag string, filter NameFilter, yes bool) error {
	ctx := cmd.Context()
	name, err := projectFor(projectFlag, nil)
	if err != nil {
		return err
	}
	tags, err := client.ProjectTags(ctx, name)
	if err != nil {
		return err
	}
	matched, err := tags.ListTagsMatching(ctx, filter)
	if err != nil {
		return err
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Name < matched[j].Name })
	if len(matched) == 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "No tags of project/%s match %s.\n", name, filter)
		return nil
	}

	if !yes {
		prompt := cmd.ErrOrStderr()
		fmt.Fprintf(prompt, "Tags of project/%s matching %s:\n", name, filter)
		for _, t := range matched {
			fmt.Fprintf(prompt, "  tag/%s\n", t.Name)
		}
		fmt.Fprintf(prompt, "Delete %d tag(s)? [y/N]: ", len(matched))
		answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Fprintln(prompt, "Aborted, nothing deleted.")
			return nil
		}
	}

	steps := make([]Step, len(matched))
	for i, t := range matched {
		steps[i] = Step{Action: ActionDelete, Server: t}
	}
	return executeDelete(ctx, tags, steps, cmd.OutOrStdout())
}

// executeDelete runs delete steps on tags, printing each once it is done; a
// tag that is already gone is reported as not found. It stops at the first
// failure. tags may be nil when no step deletes anything.
func executeDelete(ctx context.Context, tags *Tags, steps []Step, out io.Writer) error {
	for _, s := range steps {
		if s.Action == ActionDelete {
			_, err := tags.DeleteTag(ctx, s.Server.ID)
			if errors.Is(err, apiclient.ErrNotFound) {
				s.Action = ActionNotFound
			} else if err != nil {
				return fmt.Errorf("tag/%s: %w", s.Name(), err)
			}
		}
		printStep(out, s)
	}
	return nil
}

// findProjects returns those of the manifests' projects that exist on the
// server.
func findProjects(ctx context.Context, client *Client, manifests []ProjectManifest) ([]ServerProject, error) {
	var existing []ServerProject
	for _, m := range manifests {
		found, ok, err := client.FindProjectByName(ctx, m.Name)
		if err != nil {
			return nil, err
		}
		if ok {
			existing = append(existing, found)
		}
	}
	return existing, nil
}

// executeProjects runs project steps, printing each once it is done; a
// project already gone is reported as not found, one created meanwhile as
// unchanged. It stops at the first failure.
func executeProjects(ctx context.Context, client *Client, steps []ProjectStep, out io.Writer) error {
	for _, s := range steps {
		var err error
		switch s.Action {
		case ActionCreate:
			_, err = client.CreateProject(ctx, apiv0.CreateProjectRequest{Name: s.Name})
			if errors.Is(err, apiclient.ErrConflict) {
				s.Action, err = ActionUnchanged, nil // created meanwhile: the desired state is reached
			}
		case ActionDelete:
			if _, err = client.DeleteProject(ctx, s.Server.ID); errors.Is(err, apiclient.ErrNotFound) {
				s.Action, err = ActionNotFound, nil
			}
		}
		if err != nil {
			return fmt.Errorf("project/%s: %w", s.Name, err)
		}
		fmt.Fprintf(out, "project/%s %s\n", s.Name, actionResult(s.Action))
	}
	return nil
}

// applyPlan is what apply does: the projects first, then the tags of the
// project growctl works in.
type applyPlan struct {
	projects []ProjectStep
	// project is the project of the tags; "" when there are none to plan.
	project string
	tags    []Step
}

// planApply reads the manifests, fetches the relevant server state and plans
// apply. The tags are planned only when the manifests declare some or prune
// is set; the project they are in may be one the plan creates.
func planApply(ctx context.Context, client *Client, projectFlag string, files []string, stdin io.Reader, prune bool, scope NameFilter) (applyPlan, error) {
	manifests, err := ReadManifests(files, stdin)
	if err != nil {
		return applyPlan{}, err
	}
	existing, err := findProjects(ctx, client, manifests.Projects)
	if err != nil {
		return applyPlan{}, err
	}
	plan := applyPlan{projects: PlanProjects(manifests.Projects, existing, false)}
	if len(manifests.Tags) == 0 && !prune {
		return plan, nil
	}

	if plan.project, err = projectFor(projectFlag, manifests.Tags); err != nil {
		return applyPlan{}, err
	}
	var server []ServerTag
	found, ok, err := client.FindProjectByName(ctx, plan.project)
	switch {
	case err != nil:
		return applyPlan{}, err
	case ok:
		if server, err = fetchServerTags(ctx, client.TagsOf(found), manifests.Tags, prune, scope); err != nil {
			return applyPlan{}, err
		}
	case !plan.creates(plan.project):
		return applyPlan{}, fmt.Errorf("project/%s not found: declare it with kind: Project or create it first", plan.project)
	}
	if plan.tags, err = PlanApply(manifests.Tags, server, prune); err != nil {
		return applyPlan{}, err
	}
	return plan, nil
}

// creates reports whether the plan creates the project name.
func (p applyPlan) creates(name string) bool {
	for _, s := range p.projects {
		if s.Action == ActionCreate && s.Name == name {
			return true
		}
	}
	return false
}

// fetchServerTags returns the project's tags the plan depends on: the
// manifests' tags (looked up by name) plus, when pruning, the prune
// candidates — every tag of the project, or only those selected by the
// scope. The planner prunes every returned tag the manifests do not declare.
func fetchServerTags(ctx context.Context, tags *Tags, manifests []TagManifest, prune bool, scope NameFilter) ([]ServerTag, error) {
	if prune && !scope.IsSet() {
		return tags.ListTags(ctx)
	}

	var server []ServerTag
	seen := make(map[string]bool)
	if prune {
		candidates, err := tags.ListTagsMatching(ctx, scope)
		if err != nil {
			return nil, err
		}
		for _, t := range candidates {
			server = append(server, t)
			seen[t.Name] = true
		}
	}
	for _, m := range manifests {
		if seen[m.Name] {
			continue
		}
		found, ok, err := tags.FindTagByName(ctx, m.Name)
		if err != nil {
			return nil, err
		}
		if ok {
			server = append(server, found)
		}
	}
	return server, nil
}

// executeApply runs the plan, printing each step once it is done: the
// projects, then the tags. It stops at the first failure; apply is
// idempotent, so rerunning finishes the job.
func executeApply(ctx context.Context, client *Client, plan applyPlan, out io.Writer) error {
	if err := executeProjects(ctx, client, plan.projects, out); err != nil {
		return err
	}
	if plan.project == "" {
		return nil
	}
	tags, err := client.ProjectTags(ctx, plan.project)
	if err != nil {
		return err
	}
	for _, s := range plan.tags {
		var err error
		switch s.Action {
		case ActionCreate:
			err = tags.CreateTag(ctx, s.Manifest)
		case ActionPrune:
			if _, err = tags.DeleteTag(ctx, s.Server.ID); errors.Is(err, apiclient.ErrNotFound) {
				err = nil // already gone — the desired state is reached
			}
		}
		if err != nil {
			return fmt.Errorf("tag/%s: %w", s.Name(), err)
		}
		printStep(out, s)
	}
	return nil
}

func printStep(out io.Writer, s Step) {
	fmt.Fprintf(out, "tag/%s %s\n", s.Name(), actionResult(s.Action))
}

// actionResult is how a done step is reported.
func actionResult(a Action) string {
	switch a {
	case ActionCreate:
		return "created"
	case ActionUnchanged:
		return "unchanged"
	case ActionPrune:
		return "pruned"
	case ActionDelete:
		return "deleted"
	case ActionNotFound:
		return "not found"
	}
	return ""
}
