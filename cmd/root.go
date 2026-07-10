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
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "run [command [subcommand...]]",
	Short: "A CLI runtime driven by YAML command definitions",
	Long: `run is a CLI runtime: it turns commands defined in YAML files (.run.yaml)
into a command-line interface and executes them.

Bare arguments are always command names, except the single reserved name
"self", which groups run's own built-in features (run self list, run self
version, run self completion).`,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return runList(cmd)
		}
		return runCommand(cmd, args)
	},
	ValidArgsFunction: completeCommands,
}

// completeCommands returns command names for shell completion, loading
// the command file at completion time so candidates always reflect the
// current directory's .run.yaml. Already-typed arguments are resolved
// as a path through nested commands to complete the next level.
func completeCommands(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, _, err := loadConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cmds := cfg.Commands
	for _, name := range args {
		c, ok := cmds[name]
		if !ok {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cmds = c.Commands
	}
	var names []string
	for name, c := range cmds {
		if c.Description != "" {
			name += "\t" + c.Description
		}
		names = append(names, name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	// Flags must come before the command name; everything after the first
	// non-flag argument is treated as part of the command path.
	rootCmd.Flags().SetInterspersed(false)
	// Disable the default `help` and `completion` subcommands so those
	// words remain usable as command names; completion lives under
	// `run self completion` instead. Once a command has subcommands,
	// cobra insists on registering a help command and always offers it
	// in shell completion, so point it at selfCmd: it is already a real
	// subcommand, and no extra name gets reserved or completed.
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetHelpCommand(selfCmd)
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

// runCommand resolves the argument path through nested commands and
// executes the resolved command. A command without a run string lists
// its subcommands.
//
// Arguments stop being command path segments at the first "--", or at
// the first name that doesn't match a subcommand of a runnable command;
// the remainder is passed to the run string as positional parameters.
func runCommand(cmd *cobra.Command, args []string) error {
	cfg, workDir, err := loadConfig()
	if err != nil {
		return err
	}

	// SetInterspersed(false) leaves a "--" after the command name in args,
	// so split it off here rather than via ArgsLenAtDash.
	path := args
	var cmdArgs []string
	explicit := false
	if i := slices.Index(args, "--"); i >= 0 {
		path, cmdArgs, explicit = args[:i], args[i+1:], true
	}
	if len(path) == 0 {
		return runList(cmd)
	}

	cmds := cfg.Commands
	var command config.Command
	// Environment variables merge from outer to inner scopes, so
	// deeper definitions override same-named keys.
	env := make(map[string]string, len(cfg.Env))
	maps.Copy(env, cfg.Env)
	n := 0
	for n < len(path) {
		c, ok := cmds[path[n]]
		if !ok {
			break
		}
		command = c
		maps.Copy(env, c.Env)
		cmds = c.Commands
		n++
	}
	if n == 0 {
		return fmt.Errorf("command %q not found", path[0])
	}
	name := strings.Join(path[:n], " ")
	if n < len(path) {
		if explicit || command.Run == "" {
			return fmt.Errorf("command %q has no subcommand %q", name, path[n])
		}
		cmdArgs = path[n:]
	}

	if command.Run == "" {
		if len(cmdArgs) > 0 {
			return fmt.Errorf("command %q has no run", name)
		}
		return listCommands(cmd.OutOrStdout(), command.Commands, name)
	}

	cmdArgs, argEnv, err := applyArgs(command, name, cmdArgs)
	if err != nil {
		return err
	}
	maps.Copy(env, argEnv) // declared arguments have the highest precedence
	return runner.Run(command.Run, workDir, cmdArgs, envList(env), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
}

// applyArgs validates CLI arguments against the command's declared
// args, fills in defaults for missing trailing arguments, and builds an
// environment variable for each declared argument. Arguments beyond
// the declaration are passed through untouched.
func applyArgs(command config.Command, name string, args []string) ([]string, map[string]string, error) {
	if len(command.Args) == 0 {
		return args, nil, nil
	}
	final := slices.Clone(args)
	env := make(map[string]string, len(command.Args))
	for i, decl := range command.Args {
		var value string
		switch {
		case i < len(args):
			value = args[i]
		case decl.Default != nil:
			value = *decl.Default
			final = append(final, value)
		default:
			return nil, nil, fmt.Errorf("command %q: missing required argument %q", name, decl.Name)
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
