package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/longkey1/run/internal/config"
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

var selfListJSON bool

var selfListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available commands",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if selfListJSON {
			return runListJSON(cmd)
		}
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

// selfPathCmd prints run's directories so run strings and scripts can
// locate files relative to them, e.g. `. "$(run self path local)/lib.sh"`.
var selfPathCmd = &cobra.Command{
	Use:   "path [root|local|global]",
	Short: "Print a run directory path",
	Long: `Print one of run's directory paths:

  root    directory the local file's commands run in (the directory
          containing .run.yaml or .run/) — the default
  local   directory containing the local command file itself
          (.run/ for the directory form, same as root otherwise)
  global  the global config directory (~/.config/run)

root and local resolve the local command file the same way command
execution does ($RUN_CONFIG, then ancestor search) and fail when no
local file is found. global is a fixed location and is printed whether
or not it exists, but is an error while $RUN_CONFIG is set — the global
file is not used then.`,
	Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
	ValidArgs: []string{"root", "local", "global"},
	RunE: func(cmd *cobra.Command, args []string) error {
		target := "root"
		if len(args) > 0 {
			target = args[0]
		}
		path, err := selfPath(target)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), path)
		return nil
	},
}

// selfPath resolves a path target. root and local come from the local
// command file — a $RUN_CONFIG file counts as local (root is then the
// current directory, matching where its commands run); the merged
// global file never does. Results are absolute so command substitution
// output stays valid after a cd.
func selfPath(target string) (string, error) {
	if target == "global" {
		if os.Getenv("RUN_CONFIG") != "" {
			return "", fmt.Errorf("RUN_CONFIG is set; the global config directory is not used")
		}
		return config.GlobalDir()
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	files, err := config.Find(cwd)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if f.Global {
			continue
		}
		if target == "root" {
			return filepath.Abs(f.WorkDir)
		}
		return filepath.Abs(filepath.Dir(f.Path))
	}
	return "", fmt.Errorf("no local command file found (.run.yaml or .run/run.yaml)")
}

func init() {
	selfListCmd.Flags().BoolVar(&selfListJSON, "json", false, "print the full command tree as JSON")
	selfCmd.AddCommand(selfListCmd, selfVersionCmd, selfCompletionCmd, selfPathCmd)
	rootCmd.AddCommand(selfCmd)
}
