package growctl

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
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

// newLabelCommand is the assignment of labels to saved states (task 85_3;
// ADR 0025): `label assign` marks a Commit of a Project's or a Library's
// Draft and `label unassign` takes the mark off. Labels themselves are managed
// by `growctl labels`.
func newLabelCommand(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Place a label on a saved state or take it off",
		Long: "A label is the Organization's metadata; `label assign` marks one saved\n" +
			"state (Commit) of a Project or a Library and `label unassign` takes the\n" +
			"mark off (task 85_3; ADR 0025). An assignment states no Draft Version and\n" +
			"is idempotent: a repeat changes nothing. The label is named by its name,\n" +
			"ignoring case, or by its ID.",
		Args: cobra.ArbitraryArgs,
		RunE: runGroup,
	}
	cmd.AddCommand(newLabelAssignCommand(s, true), newLabelAssignCommand(s, false))
	return cmd
}

func newLabelAssignCommand(s *settings, assign bool) *cobra.Command {
	var commit int
	var label string
	var asLibrary bool
	verb, short := "assign", "Mark a saved state with a label"
	if !assign {
		verb, short = "unassign", "Take a label off a saved state"
	}
	cmd := &cobra.Command{
		Use:   verb + " <project|library> --commit <n> --label <name|id> [--library]",
		Short: short,
		Long: short + " of a Project's or a Library's Draft (task 85_3; ADR 0025):\n" +
			"the Commit is named by its number within the Draft, the label by its name\n" +
			"(ignoring case) or its ID. The name is a Project's, or a Library's with\n" +
			"--library. No Draft Version is stated.",
		Args: exactlyOneArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePositive(cmd, "commit", commit); err != nil {
				return err
			}
			if strings.TrimSpace(label) == "" {
				return usageError(fmt.Errorf("--label must name a label"))
			}
			client := s.client()
			target, err := resolveWorkflowTarget(cmd.Context(), client, args[0], asLibrary)
			if err != nil {
				return err
			}
			found, err := resolveLabel(cmd.Context(), client, label)
			if err != nil {
				return err
			}
			labels, err := target.setLabel(cmd.Context(), assign, commit, found.ID)
			if err != nil {
				return fmt.Errorf("%s commit %d: %w", target.ref(), commit, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s saved state %d: %s %s\n",
				target.ref(), commit, verb+"ed", found.Name)
			if names := labelNames(labels); names != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "labels: %s\n", names)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&commit, "commit", 0, "the number of the saved state within its Draft")
	cmd.Flags().StringVar(&label, "label", "", "the label's name (ignoring case) or ID")
	cmd.Flags().BoolVar(&asLibrary, "library", false, "the name is a Library's, not a Project's")
	return cmd
}

// resolveLabel returns the label a command names: by its name, ignoring case,
// else by its ID. A name no label holds and an ID no label has are errors.
func resolveLabel(ctx context.Context, client *Client, ref string) (apiv0.LabelResponse, error) {
	if found, ok, err := client.FindLabelByName(ctx, ref); err != nil {
		return apiv0.LabelResponse{}, err
	} else if ok {
		return found, nil
	}
	if rawID, err := uuid.Parse(ref); err == nil {
		labels, err := client.Labels(ctx)
		if err != nil {
			return apiv0.LabelResponse{}, err
		}
		for _, l := range labels {
			if l.ID == rawID {
				return l, nil
			}
		}
	}
	return apiv0.LabelResponse{}, fmt.Errorf("label/%s: not found", ref)
}

// setLabel places the label on the target's saved state, or takes it off.
func (t workflowTarget) setLabel(ctx context.Context, assign bool, commit int, labelID uuid.UUID) ([]apiv0.LabelResponse, error) {
	switch {
	case t.kind == "project" && assign:
		return t.project.AssignLabel(ctx, commit, labelID)
	case t.kind == "project":
		return t.project.UnassignLabel(ctx, commit, labelID)
	case assign:
		return t.library.AssignLabel(ctx, commit, labelID)
	default:
		return t.library.UnassignLabel(ctx, commit, labelID)
	}
}

// labelNames is the labels of a saved state as `commits` prints them: their
// names, a color in parentheses when they have one.
func labelNames(labels []apiv0.LabelResponse) string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
		if l.Color != "" {
			names[i] += " (" + l.Color + ")"
		}
	}
	return strings.Join(names, ", ")
}
