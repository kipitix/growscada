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

// AuthorEnv is the environment variable naming the default author of a Save or
// a Release, used when --author is not given.
const AuthorEnv = "GROWSCADA_AUTHOR"

// workflowTarget is what a Save, a Release or a list acts on: a Project or a
// Library, by name. A Project is looked up first because the model is
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

// save fixes the target's Draft as the next Commit under title and author.
func (t workflowTarget) save(ctx context.Context, title, author string) (apiv0.CommitResponse, error) {
	version, err := t.draftVersion(ctx)
	if err != nil {
		return apiv0.CommitResponse{}, err
	}
	req := apiv0.CreateCommitRequest{Version: version, Title: title, Author: author}
	if t.kind == "project" {
		return t.project.SaveCommit(ctx, req)
	}
	return t.library.SaveCommit(ctx, req)
}

// commits returns the target's Commit history, newest first.
func (t workflowTarget) commits(ctx context.Context) ([]apiv0.CommitResponse, error) {
	if t.kind == "project" {
		return t.project.Commits(ctx)
	}
	return t.library.Commits(ctx)
}

// releaseRow is one Release of either kind, as the lists print it.
type releaseRow struct {
	Number   int
	Commit   int // 0 for a Library Release older than Commits
	Title    string
	Note    string
	Author  string
	Origin  string
	Created time.Time
}

// releases returns the target's Releases, newest first, in one shape for a
// Project and a Library.
func (t workflowTarget) releases(ctx context.Context) ([]releaseRow, error) {
	if t.kind == "project" {
		releases, err := t.project.Releases(ctx)
		if err != nil {
			return nil, err
		}
		rows := make([]releaseRow, len(releases))
		for i, r := range releases {
			rows[i] = releaseRow{Number: r.Number, Commit: r.CommitNumber, Title: r.Title,
				Note: r.Note, Author: r.Author, Origin: r.Origin, Created: r.CreatedAt}
		}
		return rows, nil
	}
	releases, err := t.library.ListReleases(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]releaseRow, len(releases))
	for i, r := range releases {
		row := releaseRow{Number: r.Number, Note: r.Note, Author: r.Author, Origin: r.Origin, Created: r.CreatedAt}
		if r.CommitNumber != nil {
			row.Commit = *r.CommitNumber
		}
		if r.Title != nil {
			row.Title = *r.Title
		}
		rows[i] = row
	}
	return rows, nil
}

// release publishes the target's Commit as its next Release.
func (t workflowTarget) release(ctx context.Context, commit int, note, author string) (int, error) {
	if t.kind == "project" {
		release, err := t.project.CreateRelease(ctx, apiv0.CreateProjectReleaseRequest{Commit: commit, Note: note, Author: author})
		return release.Number, err
	}
	release, err := t.library.Release(ctx, apiv0.CreateReleaseRequest{Commit: commit, Note: note, Author: author})
	return release.Number, err
}

// resolveAuthor returns the author a Save or a Release records: --author, else
// GROWSCADA_AUTHOR, else the operating system's user name. An empty name is
// refused: every Commit and Release names one (ADR 0021).
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

// requireOneOf reports a missing or non-positive number flag as a usage error.
func requireOneOf(cmd *cobra.Command, flag string, value int) error {
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
		Short: "Fix the Draft as its next Commit (Save)",
		Long: "Save fixes the whole Draft of the Project or the Library as an immutable,\n" +
			"numbered Commit under a title and an author (ADR 0021). It writes no\n" +
			"DraftChange and raises no Version; a Draft that has not changed since its\n" +
			"last Commit is refused (nothing_to_save). The name is a Project's, or a\n" +
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
			fmt.Fprintf(cmd.OutOrStdout(), "%s saved commit %d\n", target.ref(), commit.Number)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "the title of the Commit")
	cmd.Flags().StringVar(&author, "author", "", "the author of the Commit (env "+AuthorEnv+")")
	cmd.Flags().BoolVar(&asLibrary, "library", false, "the name is a Library's, not a Project's")
	return cmd
}

func newReleaseCommand(s *settings) *cobra.Command {
	var commit int
	var note, author string
	var asLibrary bool
	cmd := &cobra.Command{
		Use:   "release <project|library> --commit <n> [--note <text>] [--author <name>]",
		Short: "Publish a Commit as the next Release",
		Long: "Release publishes an existing Commit as the next Release of the Project or\n" +
			"the Library: an immutable copy of the Commit's content, with the Author,\n" +
			"an optional note and the Commit's number and title (ADR 0021). It needs\n" +
			"no Draft Version and changes no Draft, so its stack survives; one Commit\n" +
			"is published at most once. The name is a Project's, or a Library's with\n" +
			"--library. Projects deploy a Release with `growctl deploy`.",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireOneOf(cmd, "commit", commit); err != nil {
				return err
			}
			author, err := resolveAuthor(author)
			if err != nil {
				return err
			}
			target, err := resolveWorkflowTarget(cmd.Context(), s.client(), args[0], asLibrary)
			if err != nil {
				return err
			}
			number, err := target.release(cmd.Context(), commit, note, author)
			if err != nil {
				return fmt.Errorf("%s: %w", target.ref(), err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s released %d\n", target.ref(), number)
			return nil
		},
	}
	cmd.Flags().IntVar(&commit, "commit", 0, "the number of the Commit to publish")
	cmd.Flags().StringVar(&note, "note", "", "the Release's note")
	cmd.Flags().StringVar(&author, "author", "", "the author of the Release (env "+AuthorEnv+")")
	cmd.Flags().BoolVar(&asLibrary, "library", false, "the name is a Library's, not a Project's")
	return cmd
}

func newDeployCommand(s *settings) *cobra.Command {
	var release int
	cmd := &cobra.Command{
		Use:   "deploy <project> --release <n>",
		Short: "Make a Release the Project's running Deployment",
		Long: "Deploy makes an existing Release the running Deployment of the Project\n" +
			"(ADR 0021, task 72_4). It takes a Release, never the Draft: the order is\n" +
			"apply → save → release → deploy. A Deploy of the Release that already\n" +
			"runs is refused (nothing_to_deploy); Operation switches to the new one.",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireOneOf(cmd, "release", release); err != nil {
				return err
			}
			target, err := resolveProjectTarget(cmd.Context(), s.client(), args[0])
			if err != nil {
				return err
			}
			deployed, err := target.project.DeployRelease(cmd.Context(), release)
			if err != nil {
				return fmt.Errorf("%s: %w", target.ref(), err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s deployed %d\n", target.ref(), deployed.Number)
			return nil
		},
	}
	cmd.Flags().IntVar(&release, "release", 0, "the number of the Release to deploy")
	return cmd
}

func newCommitsCommand(s *settings) *cobra.Command {
	var asLibrary bool
	cmd := &cobra.Command{
		Use:   "commits <project|library> [--library]",
		Short: "List the Commits of a Draft, newest first",
		Long: "List the Commits of the Project's or the Library's Draft, newest first:\n" +
			"their numbers, titles, authors, times and whether a Release published\n" +
			"each. The name is a Project's, or a Library's with --library.",
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
				fmt.Fprintf(cmd.ErrOrStderr(), "%s has no commits.\n", target.ref())
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NUMBER\tTITLE\tAUTHOR\tCREATED\tRELEASED")
			for _, c := range commits {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%t\n", c.Number, c.Title, c.Author, formatTime(c.CreatedAt), c.Released)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&asLibrary, "library", false, "the name is a Library's, not a Project's")
	return cmd
}

func newReleasesCommand(s *settings) *cobra.Command {
	var asLibrary bool
	cmd := &cobra.Command{
		Use:   "releases <project|library> [--library]",
		Short: "List the Releases of a Project or a Library, newest first",
		Long: "List the Releases of the Project or the Library, newest first: their\n" +
			"numbers, the Commit each published (or legacy for one older than\n" +
			"Commits), titles, origins, authors, times and notes. The name is a\n" +
			"Project's, or a Library's with --library.",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := resolveWorkflowTarget(cmd.Context(), s.client(), args[0], asLibrary)
			if err != nil {
				return err
			}
			releases, err := target.releases(cmd.Context())
			if err != nil {
				return fmt.Errorf("%s: %w", target.ref(), err)
			}
			if len(releases) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s has no releases.\n", target.ref())
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NUMBER\tCOMMIT\tTITLE\tORIGIN\tAUTHOR\tCREATED\tNOTE")
			for _, r := range releases {
				commit := "-"
				if r.Commit > 0 {
					commit = fmt.Sprintf("%d", r.Commit)
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Number, commit, r.Title, r.Origin, r.Author, formatTime(r.Created), r.Note)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&asLibrary, "library", false, "the name is a Library's, not a Project's")
	return cmd
}

// formatTime is how the lists print a Commit's or a Release's time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
