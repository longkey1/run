package cmd

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
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
// the remainder is passed to the run string as positional parameters,
// after declared flags are extracted from the pre-"--" portion.
func runCommand(cmd *cobra.Command, args []string) error {
	cfg, workDir, err := loadConfig()
	if err != nil {
		return err
	}

	// SetInterspersed(false) leaves a "--" after the command name in args,
	// so split it off here rather than via ArgsLenAtDash. Tokens after
	// the first "--" are always literal positionals, never flags.
	path := args
	var literal []string
	explicit := false
	if i := slices.Index(args, "--"); i >= 0 {
		path, literal, explicit = args[:i], args[i+1:], true
	}
	if len(path) == 0 {
		return runList(cmd)
	}

	cmds := cfg.Commands
	var command config.Command
	// Environment variables merge from outer to inner scopes, so
	// deeper definitions override same-named keys. Values stay
	// unevaluated until the command is known to execute, so overridden
	// dynamic entries never run.
	envVals := make(map[string]config.Value, len(cfg.Env))
	maps.Copy(envVals, cfg.Env)
	n := 0
	for n < len(path) {
		c, ok := cmds[path[n]]
		if !ok {
			break
		}
		command = c
		maps.Copy(envVals, c.Env)
		cmds = c.Commands
		n++
	}
	if n == 0 {
		return fmt.Errorf("command %q not found", path[0])
	}
	name := strings.Join(path[:n], " ")
	var rest []string // tokens after the resolved path, before any "--"
	if n < len(path) {
		if command.Run == "" {
			return fmt.Errorf("command %q has no subcommand %q", name, path[n])
		}
		rest = path[n:]
	}

	if command.Run == "" {
		if len(literal) > 0 {
			return fmt.Errorf("command %q has no run", name)
		}
		return listCommands(cmd.OutOrStdout(), command.Commands, name)
	}

	env, err := resolveEnv(envVals, workDir, name, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	// resolve evaluates a declared default. Dynamic defaults see the
	// fully resolved env, so they can reference shared values.
	resolve := func(v config.Value) (string, error) {
		if !v.IsDynamic() {
			return v.Literal, nil
		}
		return runner.Capture(v.Run, workDir, envList(env), cmd.ErrOrStderr())
	}

	positional, flagArgs, flagEnv, err := applyFlags(command, name, rest, resolve)
	if err != nil {
		return err
	}
	// With an explicit "--", everything before it must be command path
	// or declared flags.
	if explicit && len(positional) > 0 {
		return fmt.Errorf("command %q has no subcommand %q", name, positional[0])
	}
	// slices.Concat, not append: in the no-flags passthrough case
	// positional aliases the args backing array, which literal follows.
	cmdArgs, argEnv, err := applyArgs(command, name, slices.Concat(positional, literal), resolve)
	if err != nil {
		return err
	}
	cmdArgs = append(cmdArgs, flagArgs...)
	maps.Copy(env, flagEnv)
	maps.Copy(env, argEnv) // declared args and flags have the highest precedence
	return runner.Run(command.Run, workDir, cmdArgs, envList(env), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
}

// applyArgs validates CLI arguments against the command's declared
// args, fills in defaults for missing trailing arguments, and builds an
// environment variable for each declared argument. Arguments beyond
// the declaration are passed through untouched. Defaults resolve
// lazily — a dynamic default only runs when it is actually used.
func applyArgs(command config.Command, name string, args []string, resolve func(config.Value) (string, error)) ([]string, map[string]string, error) {
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
			v, err := resolve(*decl.Default)
			if err != nil {
				return nil, nil, fmt.Errorf("command %q: default for argument %q: %w", name, decl.Name, err)
			}
			value = v
			final = append(final, value)
		default:
			return nil, nil, fmt.Errorf("command %q: missing required argument %q", name, decl.Name)
		}
		env[decl.Name] = value
	}
	return final, env, nil
}

// applyFlags extracts declared long-form flags (--name, --name=value,
// --name value) from args, leaving everything else as positionals. It
// returns the positionals, the recognized flags re-normalized as
// "--name value"/"--name" tokens in declaration order (appended after
// all positionals so $1..$n stay stable and "$@" forwards everything),
// and an environment variable per declared flag: bools are
// "true"/"false", value options get the given value, their default, or
// "". Value options that are unset and have no default are omitted
// from the normalized tokens. Repeated flags: the last one wins.
// Commands without a flags declaration are passed through untouched.
// Defaults resolve lazily — a dynamic default only runs when it is
// actually used.
func applyFlags(command config.Command, name string, args []string, resolve func(config.Value) (string, error)) (positional, flagArgs []string, env map[string]string, err error) {
	if len(command.Flags) == 0 {
		return args, nil, nil, nil
	}
	decls := make(map[string]config.Flag, len(command.Flags))
	for _, f := range command.Flags {
		decls[f.Name] = f
	}
	bools := make(map[string]bool)
	values := make(map[string]string)
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if !strings.HasPrefix(tok, "--") {
			positional = append(positional, tok)
			continue
		}
		flagName, value, hasValue := strings.Cut(tok[2:], "=")
		decl, ok := decls[flagName]
		if !ok {
			return nil, nil, nil, fmt.Errorf("command %q: unknown flag --%s", name, flagName)
		}
		if decl.IsBool() {
			if hasValue {
				return nil, nil, nil, fmt.Errorf("command %q: flag --%s does not take a value", name, flagName)
			}
			bools[flagName] = true
			continue
		}
		if !hasValue {
			i++
			if i >= len(args) {
				return nil, nil, nil, fmt.Errorf("command %q: flag --%s requires a value", name, flagName)
			}
			value = args[i] // taken literally, even if it looks like a flag
		}
		values[flagName] = value
	}

	env = make(map[string]string, len(command.Flags))
	for _, decl := range command.Flags {
		if decl.IsBool() {
			set := bools[decl.Name]
			env[decl.Name] = strconv.FormatBool(set)
			if set {
				flagArgs = append(flagArgs, "--"+decl.Name)
			}
			continue
		}
		value, set := values[decl.Name]
		if !set {
			if decl.Default == nil {
				env[decl.Name] = ""
				continue
			}
			v, rerr := resolve(*decl.Default)
			if rerr != nil {
				return nil, nil, nil, fmt.Errorf("command %q: default for flag --%s: %w", name, decl.Name, rerr)
			}
			value = v // defaults materialize into "$@" like args defaults
		}
		env[decl.Name] = value
		flagArgs = append(flagArgs, "--"+decl.Name, value)
	}
	return positional, flagArgs, env, nil
}

// resolveEnv converts the merged env declarations to concrete strings,
// running dynamic values with sh in dir. A dynamic value sees the
// process environment plus the literal entries only — dynamic entries
// cannot reference one another, so no evaluation order is observable
// through the values; they still run in name order to keep any side
// effects deterministic.
func resolveEnv(env map[string]config.Value, dir, name string, stderr io.Writer) (map[string]string, error) {
	resolved := make(map[string]string, len(env))
	var dynamic []string
	for k, v := range env {
		if v.IsDynamic() {
			dynamic = append(dynamic, k)
			continue
		}
		resolved[k] = v.Literal
	}
	if len(dynamic) == 0 {
		return resolved, nil
	}
	slices.Sort(dynamic)
	literals := envList(resolved)
	for _, k := range dynamic {
		out, err := runner.Capture(env[k].Run, dir, literals, stderr)
		if err != nil {
			return nil, fmt.Errorf("command %q: env %q: %w", name, k, err)
		}
		resolved[k] = out
	}
	return resolved, nil
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
