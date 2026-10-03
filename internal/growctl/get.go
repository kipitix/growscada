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

func newGetCommand(newClient func() *Client) *cobra.Command {
	get := &cobra.Command{
		Use:   "get",
		Short: "Display server resources",
		Args:  cobra.ArbitraryArgs,
		RunE:  runGroup,
	}
	get.AddCommand(newGetTagsCommand(newClient))
	return get
}

func newGetTagsCommand(newClient func() *Client) *cobra.Command {
	var pattern, regex, output string
	cmd := &cobra.Command{
		Use:     "tags [--pattern <pattern> | --regex <regex>] [-o table|yaml]",
		Aliases: []string{"tag"},
		Short:   "List tags, optionally filtered by name",
		Long: "List tags sorted by name, optionally filtered by --pattern or --regex.\n" +
			"-o yaml prints them as a manifest (current value and quality become\n" +
			"initialValue/initialQuality), ready for apply -f.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter, err := nameFilterFromFlags(cmd, pattern, regex)
			if err != nil {
				return err
			}
			if output != "table" && output != "yaml" {
				return usageError(fmt.Errorf("unsupported output %q: use table or yaml", output))
			}

			client := newClient()
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
				fmt.Fprintln(cmd.ErrOrStderr(), "No tags found.")
				return nil
			}
			if output == "yaml" {
				return writeTagManifests(cmd.OutOrStdout(), tags)
			}
			return writeTagTable(cmd.OutOrStdout(), tags)
		},
	}
	addNameFilterFlags(cmd, &pattern, &regex, "list")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "output format: table or yaml")
	return cmd
}

func writeTagTable(out io.Writer, tags []ServerTag) error {
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tVALUE\tQUALITY\tVERSION")
	for _, t := range tags {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", t.Name, t.Type, t.Value, t.Quality, t.Version)
	}
	return w.Flush()
}

// writeTagManifests writes the tags as a multi-document manifest.
func writeTagManifests(out io.Writer, tags []ServerTag) error {
	enc := yaml.NewEncoder(out)
	enc.SetIndent(2)
	for _, t := range tags {
		var doc manifestv0.TagDocument
		doc.Header = manifestv0.Header{APIVersion: manifestv0.APIVersion(), Kind: manifestv0.KindTag}
		doc.Metadata.Name = t.Name
		doc.Spec.Type = &t.Type
		doc.Spec.InitialValue = &t.Value
		doc.Spec.InitialQuality = &t.Quality
		if err := enc.Encode(doc); err != nil {
			return err
		}
	}
	return enc.Close()
}
