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

// NewRootCommand builds the growctl command tree. Output goes to the command's
// out writer (cmd.SetOut), manifests given as "-" are read from its input.
func NewRootCommand() *cobra.Command {
	server := os.Getenv(ServerEnv)
	if server == "" {
		server = DefaultServer
	}

	root := &cobra.Command{
		Use:           "growctl",
		Short:         "Declaratively manage GrowSCADA tags from YAML manifests",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Usage errors exit with 2, so diff's exit status 1 always means "there are changes".
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &ExitError{Code: 2, Err: err}
	})
	root.PersistentFlags().StringVar(&server, "server", server,
		"GrowSCADA REST API URL (env "+ServerEnv+")")

	newClient := func() *Client {
		return NewClient(server, &http.Client{Timeout: 30 * time.Second})
	}

	root.AddCommand(
		newApplyCommand(newClient),
		newDiffCommand(newClient),
		newDeleteCommand(newClient),
		newGetCommand(newClient),
	)
	return root
}

// noArgs rejects positional arguments as a usage error (exit status 2).
func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return &ExitError{Code: 2, Err: err}
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

func newApplyCommand(newClient func() *Client) *cobra.Command {
	var (
		files []string
		flags pruneFlags
	)
	cmd := &cobra.Command{
		Use:   "apply -f <file|directory|->",
		Short: "Create the manifest's tags that are missing on the server",
		Long: "Create the manifest's tags that are missing on the server. initialValue and\n" +
			"initialQuality are used only on creation; existing tags keep their value.\n" +
			"With --prune, server tags absent from the manifest are deleted; --pattern or\n" +
			"--regex limits pruning to the tags they select (the manifest's own scope).",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, err := flags.scope(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			client := newClient()
			steps, err := planApply(ctx, client, files, cmd.InOrStdin(), flags.prune, scope)
			if err != nil {
				return err
			}
			return executeApply(ctx, client, steps, cmd.OutOrStdout())
		},
	}
	addFileFlag(cmd, &files)
	addPruneFlags(cmd, &flags, "delete server tags absent from the manifest")
	return cmd
}

func newDiffCommand(newClient func() *Client) *cobra.Command {
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
			scope, err := flags.scope(cmd)
			if err != nil {
				return err
			}
			steps, err := planApply(cmd.Context(), newClient(), files, cmd.InOrStdin(), flags.prune, scope)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			out := cmd.OutOrStdout()
			changed := false
			for _, s := range steps {
				switch s.Action {
				case ActionCreate:
					fmt.Fprintf(out, "+ tag/%s (%s)\n", s.Name(), s.Manifest.Type)
				case ActionPrune:
					fmt.Fprintf(out, "- tag/%s\n", s.Name())
				}
				changed = changed || s.IsChange()
			}
			if changed {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	addFileFlag(cmd, &files)
	addPruneFlags(cmd, &flags, "include server tags absent from the manifest")
	return cmd
}

func newDeleteCommand(newClient func() *Client) *cobra.Command {
	var (
		files          []string
		pattern, regex string
		yes            bool
	)
	cmd := &cobra.Command{
		Use:   "delete (-f <file|directory|-> | --pattern <pattern> | --regex <regex>)",
		Short: "Delete the manifest's tags, or the tags matching a pattern",
		Long: "Delete tags by name: those declared in the manifest (-f; the spec is ignored,\n" +
			"missing tags are reported as not found), or those matching --pattern/--regex.\n" +
			"Deleting by pattern lists the tags and asks for confirmation unless --yes is given.",
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
				return deleteMatching(cmd, newClient(), filter, yes)
			case len(files) == 0:
				return usageError(errors.New("nothing to delete: use -f <file|directory|-> or --pattern/--regex"))
			}
			return deleteManifests(cmd, newClient(), files)
		},
	}
	addFileFlag(cmd, &files)
	addNameFilterFlags(cmd, &pattern, &regex, "delete")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "delete by pattern without asking for confirmation")
	return cmd
}

// deleteManifests deletes the manifests' tags by name.
func deleteManifests(cmd *cobra.Command, client *Client, files []string) error {
	ctx := cmd.Context()
	manifests, err := ReadManifests(files, cmd.InOrStdin())
	if err != nil {
		return err
	}
	server, err := fetchServerTags(ctx, client, manifests, false, NameFilter{})
	if err != nil {
		return err
	}
	return executeDelete(ctx, client, PlanDelete(manifests, server), cmd.OutOrStdout())
}

// deleteMatching deletes the server tags selected by the filter, after the
// user confirms the list (prompt on stderr, answer from stdin) unless yes.
func deleteMatching(cmd *cobra.Command, client *Client, filter NameFilter, yes bool) error {
	ctx := cmd.Context()
	matched, err := client.ListTagsMatching(ctx, filter)
	if err != nil {
		return err
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Name < matched[j].Name })
	if len(matched) == 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "No tags match %s.\n", filter)
		return nil
	}

	if !yes {
		prompt := cmd.ErrOrStderr()
		fmt.Fprintf(prompt, "Tags matching %s:\n", filter)
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
	return executeDelete(ctx, client, steps, cmd.OutOrStdout())
}

// executeDelete runs delete steps, printing each once it is done; a tag that
// is already gone is reported as not found. It stops at the first failure.
func executeDelete(ctx context.Context, client *Client, steps []Step, out io.Writer) error {
	for _, s := range steps {
		if s.Action == ActionDelete {
			err := client.DeleteTag(ctx, s.Server.ID)
			if errors.Is(err, errNotFound) {
				s.Action = ActionNotFound
			} else if err != nil {
				return fmt.Errorf("tag/%s: %w", s.Name(), err)
			}
		}
		printStep(out, s)
	}
	return nil
}

// planApply reads the manifests, fetches the relevant server state and plans apply.
func planApply(ctx context.Context, client *Client, files []string, stdin io.Reader, prune bool, scope NameFilter) ([]Step, error) {
	manifests, err := ReadManifests(files, stdin)
	if err != nil {
		return nil, err
	}
	server, err := fetchServerTags(ctx, client, manifests, prune, scope)
	if err != nil {
		return nil, err
	}
	return PlanApply(manifests, server, prune)
}

// fetchServerTags returns the server tags the plan depends on: the manifests'
// tags (looked up by name) plus, when pruning, the prune candidates — every
// server tag, or only those selected by the scope. The planner prunes every
// returned tag the manifests do not declare.
func fetchServerTags(ctx context.Context, client *Client, manifests []TagManifest, prune bool, scope NameFilter) ([]ServerTag, error) {
	if prune && !scope.IsSet() {
		return client.ListTags(ctx)
	}

	var server []ServerTag
	seen := make(map[string]bool)
	if prune {
		candidates, err := client.ListTagsMatching(ctx, scope)
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
		found, ok, err := client.FindTagByName(ctx, m.Name)
		if err != nil {
			return nil, err
		}
		if ok {
			server = append(server, found)
		}
	}
	return server, nil
}

// executeApply runs the plan, printing each step once it is done. It stops at
// the first failure; apply is idempotent, so rerunning finishes the job.
func executeApply(ctx context.Context, client *Client, steps []Step, out io.Writer) error {
	for _, s := range steps {
		var err error
		switch s.Action {
		case ActionCreate:
			err = client.CreateTag(ctx, s.Manifest)
		case ActionPrune:
			if err = client.DeleteTag(ctx, s.Server.ID); errors.Is(err, errNotFound) {
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
	var result string
	switch s.Action {
	case ActionCreate:
		result = "created"
	case ActionUnchanged:
		result = "unchanged"
	case ActionPrune:
		result = "pruned"
	case ActionDelete:
		result = "deleted"
	case ActionNotFound:
		result = "not found"
	}
	fmt.Fprintf(out, "tag/%s %s\n", s.Name(), result)
}
