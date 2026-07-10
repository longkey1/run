package cmd

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List available tasks",
	Long:    `List all tasks defined in the task file.`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(cmd)
	},
}

func runList(cmd *cobra.Command) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}

	names := make([]string, 0, len(cfg.Tasks))
	for name := range cfg.Tasks {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, name := range names {
		desc := cfg.Tasks[name].Description
		if desc == "" {
			fmt.Fprintf(w, "  %s\n", name)
		} else {
			fmt.Fprintf(w, "  %s\t- %s\n", name, desc)
		}
	}
	return w.Flush()
}

func init() {
	rootCmd.AddCommand(listCmd)
}
