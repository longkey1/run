package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/longkey1/run/internal/config"
	"github.com/longkey1/run/internal/runner"
	"github.com/longkey1/run/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "run [flags] [task [subtask...]]",
	Short: "A simple task runner",
	Long: `run is a simple task runner that executes tasks defined in YAML files (.run.yaml).

Arguments without flags are always task names; run's own features are
exposed only through flags, so any task name can be used freely.`,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if v, _ := cmd.Flags().GetBool("version"); v {
			fmt.Fprintln(cmd.OutOrStdout(), version.Info())
			return nil
		}
		if shell, _ := cmd.Flags().GetString("completion"); shell != "" {
			return genCompletion(cmd, shell)
		}
		if l, _ := cmd.Flags().GetBool("list"); l || len(args) == 0 {
			return runList(cmd)
		}
		return runTask(cmd, args)
	},
}

func init() {
	rootCmd.Flags().BoolP("list", "l", false, "List available tasks")
	rootCmd.Flags().Bool("version", false, "Show version information")
	rootCmd.Flags().String("completion", "", "Generate shell completion script (bash|zsh|fish|powershell)")
	// Flags must come before the task name; everything after the first
	// non-flag argument is treated as part of the task path.
	rootCmd.Flags().SetInterspersed(false)
	// Disable the default `help` and `completion` subcommands so those
	// words remain usable as task names.
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{Use: "_help", Hidden: true})
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exitErr *runner.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(1)
	}
}

func loadConfig() (*config.Config, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	path, workDir, err := config.Find(cwd)
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, "", err
	}
	return cfg, workDir, nil
}

// runTask resolves the argument path through nested tasks and executes
// the resolved task. A task without a command lists its subtasks.
func runTask(cmd *cobra.Command, args []string) error {
	cfg, workDir, err := loadConfig()
	if err != nil {
		return err
	}

	tasks := cfg.Tasks
	var task config.Task
	for i, name := range args {
		t, ok := tasks[name]
		if !ok {
			if i == 0 {
				return fmt.Errorf("task %q not found", name)
			}
			return fmt.Errorf("task %q has no subtask %q", strings.Join(args[:i], " "), name)
		}
		task = t
		tasks = t.Tasks
	}

	if task.Command == "" {
		return listTasks(cmd.OutOrStdout(), task.Tasks, strings.Join(args, " "))
	}
	return runner.Run(task.Command, workDir, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
}

func genCompletion(cmd *cobra.Command, shell string) error {
	root := cmd.Root()
	out := cmd.OutOrStdout()
	switch shell {
	case "bash":
		return root.GenBashCompletionV2(out, true)
	case "zsh":
		return root.GenZshCompletion(out)
	case "fish":
		return root.GenFishCompletion(out, true)
	case "powershell":
		return root.GenPowerShellCompletionWithDesc(out)
	default:
		return fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish, powershell)", shell)
	}
}
