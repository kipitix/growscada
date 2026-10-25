package growctl

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kipitix/growscada/internal/server/domain/project"
)

func newConfigCommand() *cobra.Command {
	config := &cobra.Command{
		Use:   "config",
		Short: "Manage growctl's context: the project it works in",
		Long: "Manage growctl's context, kept in a file between runs (env " + ConfigEnv + ",\n" +
			"default growctl/config.yaml in the user's configuration directory).",
		Args: cobra.ArbitraryArgs,
		RunE: runGroup,
	}
	config.AddCommand(newUseProjectCommand(), newCurrentProjectCommand())
	return config
}

func newUseProjectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use-project <name>",
		Short: "Work in the given project when --project is not given",
		Long: "Work in the given project when --project is not given. The name is not\n" +
			"looked up on the server: a missing project is reported when it is used.",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return usageError(err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := project.NewProjectName(args[0]); err != nil {
				return usageError(fmt.Errorf("invalid project %q: %w", args[0], err))
			}
			c, err := readContext()
			if err != nil {
				return err
			}
			c.Project = args[0]
			if _, err := writeContext(c); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Working in project/%s.\n", args[0])
			return nil
		},
	}
}

func newCurrentProjectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "current-project",
		Short: "Print the project of the context",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := readContext()
			if err != nil {
				return err
			}
			if c.Project == "" {
				return errors.New("no project in the context: use growctl config use-project <name>")
			}
			fmt.Fprintln(cmd.OutOrStdout(), c.Project)
			return nil
		},
	}
}
