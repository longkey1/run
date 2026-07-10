package cmd

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
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
	ValidArgsFunction: completeTasks,
}

// completeTasks returns task names for shell completion, loading the
// task file at completion time so candidates always reflect the
// current directory's .run.yaml. Already-typed arguments are resolved
// as a path through nested tasks to complete the next level.
func completeTasks(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, _, err := loadConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	tasks := cfg.Tasks
	for _, name := range args {
		t, ok := tasks[name]
		if !ok {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		tasks = t.Tasks
	}
	var names []string
	for name, t := range tasks {
		if t.Description != "" {
			name += "\t" + t.Description
		}
		names = append(names, name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
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
//
// Arguments stop being task path segments at the first "--", or at the
// first name that doesn't match a subtask of a command-bearing task;
// the remainder is passed to the command as positional parameters.
func runTask(cmd *cobra.Command, args []string) error {
	cfg, workDir, err := loadConfig()
	if err != nil {
		return err
	}

	// SetInterspersed(false) leaves a "--" after the task name in args,
	// so split it off here rather than via ArgsLenAtDash.
	path := args
	var taskArgs []string
	explicit := false
	if i := slices.Index(args, "--"); i >= 0 {
		path, taskArgs, explicit = args[:i], args[i+1:], true
	}
	if len(path) == 0 {
		return runList(cmd)
	}

	tasks := cfg.Tasks
	var task config.Task
	// Environment variables merge from outer to inner scopes, so
	// deeper definitions override same-named keys.
	env := make(map[string]string, len(cfg.Env))
	maps.Copy(env, cfg.Env)
	n := 0
	for n < len(path) {
		t, ok := tasks[path[n]]
		if !ok {
			break
		}
		task = t
		maps.Copy(env, t.Env)
		tasks = t.Tasks
		n++
	}
	if n == 0 {
		return fmt.Errorf("task %q not found", path[0])
	}
	name := strings.Join(path[:n], " ")
	if n < len(path) {
		if explicit || task.Command == "" {
			return fmt.Errorf("task %q has no subtask %q", name, path[n])
		}
		taskArgs = path[n:]
	}

	if task.Command == "" {
		if len(taskArgs) > 0 {
			return fmt.Errorf("task %q has no command", name)
		}
		return listTasks(cmd.OutOrStdout(), task.Tasks, name)
	}

	taskArgs, argEnv, err := applyArgs(task, name, taskArgs)
	if err != nil {
		return err
	}
	maps.Copy(env, argEnv) // declared arguments have the highest precedence
	return runner.Run(task.Command, workDir, taskArgs, envList(env), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
}

// applyArgs validates CLI arguments against the task's declared args,
// fills in defaults for missing trailing arguments, and builds an
// environment variable for each declared argument. Arguments beyond
// the declaration are passed through untouched.
func applyArgs(task config.Task, name string, args []string) ([]string, map[string]string, error) {
	if len(task.Args) == 0 {
		return args, nil, nil
	}
	final := slices.Clone(args)
	env := make(map[string]string, len(task.Args))
	for i, decl := range task.Args {
		var value string
		switch {
		case i < len(args):
			value = args[i]
		case decl.Default != nil:
			value = *decl.Default
			final = append(final, value)
		default:
			return nil, nil, fmt.Errorf("task %q: missing required argument %q", name, decl.Name)
		}
		env[decl.Name] = value
	}
	return final, env, nil
}

// envList converts an env map to sorted "name=value" pairs so the
// constructed environment is deterministic. Keys are unique, so
// sorting the joined pairs sorts by name.
func envList(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	list := make([]string, 0, len(env))
	for name, value := range env {
		list = append(list, name+"="+value)
	}
	slices.Sort(list)
	return list
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
