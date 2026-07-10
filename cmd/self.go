package cmd

import (
	"fmt"

	"github.com/longkey1/run/internal/version"
	"github.com/spf13/cobra"
)

// selfCmd groups run's own built-in features under the single reserved
// name "self", so every other bare argument remains a user-defined
// command name. Config loading rejects a top-level command named
// "self".
var selfCmd = &cobra.Command{
	Use:   "self",
	Short: "run's own built-in commands",
}

var selfListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available commands",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(cmd)
	},
}

var selfVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), version.Info())
	},
}

var selfCompletionCmd = &cobra.Command{
	Use:       "completion <shell>",
	Short:     "Generate shell completion script",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return genCompletion(cmd, args[0])
	},
}

func init() {
	selfCmd.AddCommand(selfListCmd, selfVersionCmd, selfCompletionCmd)
	rootCmd.AddCommand(selfCmd)
}
