package growctl

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	manifestv0 "github.com/kipitix/growscada/contract/manifest/v0"
)

func newGetCommand(s *settings) *cobra.Command {
	get := &cobra.Command{
		Use:   "get",
		Short: "Display server resources",
		Args:  cobra.ArbitraryArgs,
		RunE:  runGroup,
	}
	get.AddCommand(newGetProjectsCommand(s), newGetTagsCommand(s))
	return get
}

// checkOutput rejects an -o value other than table or yaml (exit status 2).
func checkOutput(output string) error {
	if output != "table" && output != "yaml" {
		return usageError(fmt.Errorf("unsupported output %q: use table or yaml", output))
	}
	return nil
}

func newGetProjectsCommand(s *settings) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:     "projects [-o table|yaml]",
		Aliases: []string{"project"},
		Short:   "List projects",
		Long: "List the server's projects in creation order. -o yaml prints them as a\n" +
			"manifest, ready for apply -f.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkOutput(output); err != nil {
				return err
			}
			projects, err := s.client().ListProjects(cmd.Context())
			if err != nil {
				return err
			}
			if len(projects) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "No projects found.")
				return nil
			}
			if output == "yaml" {
				return writeProjectManifests(cmd.OutOrStdout(), projects)
			}
			return writeProjectTable(cmd.OutOrStdout(), projects)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "table", "output format: table or yaml")
	return cmd
}

func newGetTagsCommand(s *settings) *cobra.Command {
	var pattern, regex, output string
	cmd := &cobra.Command{
		Use:     "tags [--pattern <pattern> | --regex <regex>] [-o table|yaml]",
		Aliases: []string{"tag"},
		Short:   "List the tags of the project, optionally filtered by name",
		Long: "List the tags of the project growctl works in (--project) sorted by name,\n" +
			"optionally filtered by --pattern or --regex. -o yaml prints them as a\n" +
			"manifest (current value and quality become initialValue/initialQuality,\n" +
			"the project metadata.project), ready for apply -f.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter, err := nameFilterFromFlags(cmd, pattern, regex)
			if err != nil {
				return err
			}
			if err := checkOutput(output); err != nil {
				return err
			}
			name, err := projectFor(s.project, nil)
			if err != nil {
				return err
			}

			client, err := s.client().ProjectTags(cmd.Context(), name)
			if err != nil {
				return err
			}
			var tags []ServerTag
			if filter.IsSet() {
				tags, err = client.ListTagsMatching(cmd.Context(), filter)
			} else {
				tags, err = client.ListTags(cmd.Context())
			}
			if err != nil {
				return err
			}
			sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })

			if len(tags) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "No tags found in project/%s.\n", name)
				return nil
			}
			if output == "yaml" {
				return writeTagManifests(cmd.OutOrStdout(), name, tags)
			}
			return writeTagTable(cmd.OutOrStdout(), tags)
		},
	}
	addNameFilterFlags(cmd, &pattern, &regex, "list")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "output format: table or yaml")
	return cmd
}

func writeProjectTable(out io.Writer, projects []ServerProject) error {
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tVERSION")
	for _, p := range projects {
		fmt.Fprintf(w, "%s\t%s\t%d\n", p.Name, p.ID, p.Version)
	}
	return w.Flush()
}

// writeProjectManifests writes the projects as a multi-document manifest.
func writeProjectManifests(out io.Writer, projects []ServerProject) error {
	enc := yaml.NewEncoder(out)
	enc.SetIndent(2)
	for _, p := range projects {
		var doc manifestv0.ProjectDocument
		doc.Header = manifestv0.Header{APIVersion: manifestv0.APIVersion(), Kind: manifestv0.KindProject}
		doc.Metadata.Name = p.Name
		if err := enc.Encode(doc); err != nil {
			return err
		}
	}
	return enc.Close()
}

func writeTagTable(out io.Writer, tags []ServerTag) error {
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tVALUE\tQUALITY\tVERSION")
	for _, t := range tags {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", t.Name, t.Type, t.Value, t.Quality, t.Version)
	}
	return w.Flush()
}

// writeTagManifests writes the tags of the project as a multi-document
// manifest.
func writeTagManifests(out io.Writer, project string, tags []ServerTag) error {
	enc := yaml.NewEncoder(out)
	enc.SetIndent(2)
	for _, t := range tags {
		var doc manifestv0.TagDocument
		doc.Header = manifestv0.Header{APIVersion: manifestv0.APIVersion(), Kind: manifestv0.KindTag}
		doc.Metadata.Name = t.Name
		doc.Metadata.Project = project
		doc.Spec.Type = &t.Type
		doc.Spec.InitialValue = &t.Value
		doc.Spec.InitialQuality = &t.Quality
		if err := enc.Encode(doc); err != nil {
			return err
		}
	}
	return enc.Close()
}
