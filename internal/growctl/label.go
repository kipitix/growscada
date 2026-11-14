package growctl

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// newLabelsCommand is the minimal label management of task 85_2: the labels
// are metadata of the Organization, not declarative content, so there is no
// label manifest kind (ADR 0025). A label is named by its name here; only the
// API names it by ID.
func newLabelsCommand(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "labels",
		Short: "List, create, rename, recolor and delete labels",
		Long: "Labels are named marks with an optional #RRGGBB color that Engineers put on\n" +
			"saved states to group and filter them (task 85_2; ADR 0025). They belong to the\n" +
			"server's Organization, not to a Project or a Library, and they are not content:\n" +
			"no manifest, saved state or Deployment carries them. `growctl labels` lists\n" +
			"them; the subcommands change one of them.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listLabels(cmd, s)
		},
	}
	cmd.AddCommand(
		newLabelsCreateCommand(s),
		newLabelsRenameCommand(s),
		newLabelsColorCommand(s),
		newLabelsDeleteCommand(s),
	)
	return cmd
}

func listLabels(cmd *cobra.Command, s *settings) error {
	labels, err := s.client().Labels(cmd.Context())
	if err != nil {
		return err
	}
	if len(labels) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "No labels found.")
		return nil
	}
	return writeLabelTable(cmd.OutOrStdout(), labels)
}

func newLabelsCreateCommand(s *settings) *cobra.Command {
	var color string
	cmd := &cobra.Command{
		Use:   "create <name> [--color #RRGGBB]",
		Short: "Create a label",
		Long: "Create a label with a name, unique in the Organization without regard to case,\n" +
			"and an optional #RRGGBB color; no color is the theme's neutral one.",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			created, err := s.client().CreateLabel(cmd.Context(), args[0], color)
			if err != nil {
				return fmt.Errorf("label/%s: %w", args[0], err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "label/%s created\n", created.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&color, "color", "", "label color as #RRGGBB (default: none)")
	return cmd
}

func newLabelsRenameCommand(s *settings) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a label",
		Long:  "Rename a label; the name is unique in the Organization without regard to case.",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return usageError(err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client := s.client()
			found, ok, err := client.FindLabelByName(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("label/%s: not found", args[0])
			}
			name := args[1]
			updated, err := client.UpdateLabel(cmd.Context(), found.ID, &name, nil)
			if err != nil {
				return fmt.Errorf("label/%s: %w", args[1], err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "label/%s renamed to %s\n", found.Name, updated.Name)
			return nil
		},
	}
}

func newLabelsColorCommand(s *settings) *cobra.Command {
	return &cobra.Command{
		Use:   "color <name> <#RRGGBB|none>",
		Short: "Change the color of a label",
		Long:  "Change the color of a label; `none` clears it to the theme's neutral color.",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return usageError(err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			color := args[1]
			if strings.EqualFold(color, "none") {
				color = ""
			}
			client := s.client()
			found, ok, err := client.FindLabelByName(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("label/%s: not found", args[0])
			}
			updated, err := client.UpdateLabel(cmd.Context(), found.ID, nil, &color)
			if err != nil {
				return fmt.Errorf("label/%s: %w", args[0], err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "label/%s colored %s\n", updated.Name, colorOrNone(updated.Color))
			return nil
		},
	}
}

func newLabelsDeleteCommand(s *settings) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a label with its assignments",
		Long:  "Delete a label; the saved states it marks lose the mark. The label is not restored by a later migration.",
		Args:  exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := s.client()
			found, ok, err := client.FindLabelByName(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("label/%s: not found", args[0])
			}
			if err := client.DeleteLabel(cmd.Context(), found.ID); err != nil {
				return fmt.Errorf("label/%s: %w", found.Name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "label/%s deleted\n", found.Name)
			return nil
		},
	}
}

func writeLabelTable(out io.Writer, labels []apiv0.LabelResponse) error {
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tCOLOR\tASSIGNED")
	for _, l := range labels {
		fmt.Fprintf(w, "%s\t%s\t%d\n", l.Name, colorOrNone(l.Color), l.AssignedCount)
	}
	return w.Flush()
}

// colorOrNone is a label's color in the table: "-" when it has none.
func colorOrNone(color string) string {
	if color == "" {
		return "-"
	}
	return color
}
