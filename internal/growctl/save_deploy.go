package growctl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// AuthorEnv is the environment variable naming the default author of a Save,
// used when --author is not given.
const AuthorEnv = "GROWSCADA_AUTHOR"

// workflowTarget is what a Save or a list acts on: a Project or a Library, by
// name. A Project is looked up first because the model is
// Project-first; --library forces the other reading when a Library has the
// same name as a Project.
type workflowTarget struct {
	kind    string // "project" or "library"
	name    string
	project *apiclient.ProjectClient
	library *apiclient.LibraryClient
}

// ref is how the target is named in output and errors: project/Main.
func (t workflowTarget) ref() string { return t.kind + "/" + t.name }

// resolveProjectTarget returns the Project with the given name.
func resolveProjectTarget(ctx context.Context, client *Client, name string) (workflowTarget, error) {
	found, ok, err := client.FindProjectByName(ctx, name)
	if err != nil {
		return workflowTarget{}, err
	}
	if !ok {
		return workflowTarget{}, fmt.Errorf("project/%s not found", name)
	}
	return workflowTarget{kind: "project", name: name, project: client.Project(found.ID)}, nil
}

// resolveWorkflowTarget returns the Project or the Library with the given
// name: the Project unless asLibrary is set; without it, a name no Project has
// is looked up among the Libraries.
func resolveWorkflowTarget(ctx context.Context, client *Client, name string, asLibrary bool) (workflowTarget, error) {
	if asLibrary {
		lib, _, err := findLibrary(ctx, client, name)
		if err != nil {
			return workflowTarget{}, err
		}
		return workflowTarget{kind: "library", name: name, library: lib}, nil
	}
	project, ok, err := client.FindProjectByName(ctx, name)
	if err != nil {
		return workflowTarget{}, err
	}
	if ok {
		return workflowTarget{kind: "project", name: name, project: client.Project(project.ID)}, nil
	}
	lib, ok, err := client.FindLibraryByName(ctx, name)
	if err != nil {
		return workflowTarget{}, err
	}
	if ok {
		return workflowTarget{kind: "library", name: name, library: client.Library(lib.ID)}, nil
	}
	return workflowTarget{}, fmt.Errorf("%s not found: no project or library has this name", name)
}

// draftVersion reads the Version of the target's Draft, the token a Save
// states (ADR 0016).
func (t workflowTarget) draftVersion(ctx context.Context) (int, error) {
	if t.kind == "project" {
		state, err := t.project.DraftState(ctx)
		return state.Version, err
	}
	state, err := t.library.DraftState(ctx)
	return state.Version, err
}

// save fixes the target's Draft as the next saved state under title and author.
func (t workflowTarget) save(ctx context.Context, title, author string) (apiv0.SavedStateResponse, error) {
	version, err := t.draftVersion(ctx)
	if err != nil {
		return apiv0.SavedStateResponse{}, err
	}
	req := apiv0.CreateSavedStateRequest{Version: version, Title: title, Author: author}
	if t.kind == "project" {
		return t.project.SaveSavedState(ctx, req)
	}
	return t.library.SaveSavedState(ctx, req)
}

// commits returns the target's saved state history, newest first.
func (t workflowTarget) commits(ctx context.Context) ([]apiv0.SavedStateResponse, error) {
	if t.kind == "project" {
		return t.project.SavedStates(ctx)
	}
	return t.library.SavedStates(ctx)
}

// resolveAuthor returns the author a Save records: --author, else
// GROWSCADA_AUTHOR, else the operating system's user name. An empty name is
// refused: every saved state names one (ADR 0021).
func resolveAuthor(flag string) (string, error) {
	if author := strings.TrimSpace(flag); author != "" {
		return author, nil
	}
	if author := strings.TrimSpace(os.Getenv(AuthorEnv)); author != "" {
		return author, nil
	}
	if current, err := user.Current(); err == nil {
		if author := strings.TrimSpace(current.Username); author != "" {
			return author, nil
		}
	}
	return "", usageError(fmt.Errorf("no author: use --author or set %s", AuthorEnv))
}

// requirePositive reports a missing or non-positive number flag as a usage error.
func requirePositive(cmd *cobra.Command, flag string, value int) error {
	if value > 0 {
		return nil
	}
	return usageError(fmt.Errorf("--%s must be a positive number", flag))
}

func newSaveCommand(s *settings) *cobra.Command {
	var title, author string
	var asLibrary bool
	cmd := &cobra.Command{
		Use:   "save <project|library> --title <title> [--author <name>]",
		Short: "Fix the Draft as its next saved state (Save)",
		Long: "Save fixes the whole Draft of the Project or the Library as an immutable,\n" +
			"numbered saved state under a title and an author (ADR 0021). It writes no\n" +
			"DraftChange and raises no Version; a Draft that has not changed since its\n" +
			"last saved state is refused (nothing_to_save). The name is a Project's, or a\n" +
			"Library's with --library. The author is --author, else GROWSCADA_AUTHOR,\n" +
			"else the operating system's user name.",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(title) == "" {
				return usageError(errors.New("save needs a non-empty --title"))
			}
			author, err := resolveAuthor(author)
			if err != nil {
				return err
			}
			target, err := resolveWorkflowTarget(cmd.Context(), s.client(), args[0], asLibrary)
			if err != nil {
				return err
			}
			commit, err := target.save(cmd.Context(), title, author)
			if err != nil {
				return fmt.Errorf("%s: %w", target.ref(), err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s saved state %d\n", target.ref(), commit.Number)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "the title of the saved state")
	cmd.Flags().StringVar(&author, "author", "", "the author of the saved state (env "+AuthorEnv+")")
	cmd.Flags().BoolVar(&asLibrary, "library", false, "the name is a Library's, not a Project's")
	return cmd
}

func newDeployCommand(s *settings) *cobra.Command {
	var commit int
	cmd := &cobra.Command{
		Use:   "deploy <project> --commit <n>",
		Short: "Make a saved state the Project's running Deployment",
		Long: "Deploy makes an existing saved state the running Deployment\n" +
			"of the Project (ADR 0021, task 85_4). It takes a saved state, never the Draft:\n" +
			"the order is apply → save → deploy. A Deploy of the saved state that\n" +
			"already runs is refused (nothing_to_deploy); Operation switches to the new\n" +
			"one.",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositive(cmd, "commit", commit); err != nil {
				return err
			}
			target, err := resolveProjectTarget(cmd.Context(), s.client(), args[0])
			if err != nil {
				return err
			}
			deployed, err := target.project.DeploySavedState(cmd.Context(), commit)
			if err != nil {
				return fmt.Errorf("%s: %w", target.ref(), err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s deployed %d\n", target.ref(), deployed.Number)
			return nil
		},
	}
	cmd.Flags().IntVar(&commit, "commit", 0, "the number of the saved state to deploy")
	return cmd
}

func newCommitsCommand(s *settings) *cobra.Command {
	var asLibrary bool
	cmd := &cobra.Command{
		Use:   "commits <project|library> [--library]",
		Short: "List the saved states of a Draft, newest first",
		Long: "List the saved states of the Project's or the Library's Draft, newest first:\n" +
			"their numbers, titles, authors, times and labels. The name is a Project's,\n" +
			"or a Library's with --library.",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := resolveWorkflowTarget(cmd.Context(), s.client(), args[0], asLibrary)
			if err != nil {
				return err
			}
			commits, err := target.commits(cmd.Context())
			if err != nil {
				return fmt.Errorf("%s: %w", target.ref(), err)
			}
			if len(commits) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s has no saved states.\n", target.ref())
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NUMBER\tTITLE\tAUTHOR\tCREATED\tLABELS")
			for _, c := range commits {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", c.Number, c.Title, c.Author, formatTime(c.CreatedAt), labelNames(c.Labels))
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&asLibrary, "library", false, "the name is a Library's, not a Project's")
	return cmd
}

// formatTime is how the lists print a saved state's time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
