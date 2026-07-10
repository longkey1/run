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
	Long: `run is a CLI runtime: it turns commands defined in YAML files (.run.yaml,
or .run/run.yaml) into a command-line interface and executes them.

Bare arguments are always command names, except the single reserved name
"self", which groups run's own built-in features (run self list, run self
version, run self completion).

"run <command> --help" shows a command's declared arguments and options;
use "run <command> -- --help" to pass a literal --help through instead.`,
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

// completeCommands returns shell completion candidates, loading the
// command files at completion time so they always reflect the current
// directory's .run.yaml plus the merged global file. Already-typed
// arguments are resolved as a
// path through nested commands, mirroring runCommand's greedy
// resolution: while still on the path it completes the next level's
// command names; past it, a word starting with "-" completes the
// resolved command's declared options. Tokens after a literal "--",
// option-value positions, and positional positions get no candidates,
// and nothing ever falls back to file completion.
func completeCommands(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	files, err := loadConfigFiles()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	// Tokens after a literal "--" are always literal positionals.
	if slices.Contains(args, "--") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cmds := mergedCommands(files)
	var command config.Command
	n := 0
	for n < len(args) {
		c, ok := cmds[args[n]]
		if !ok {
			break
		}
		command = c
		cmds = c.Commands
		n++
	}
	rest := args[n:] // already-typed options/positionals, never path segments
	// The word being completed is a value option's pending value, which
	// runCommand takes literally even if it looks like an option.
	if awaitingOptionValue(command, rest) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if strings.HasPrefix(toComplete, "-") {
		if n == 0 || strings.Contains(toComplete, "=") {
			// No command resolved yet, or the cursor is in the value
			// part of --name=: nothing to suggest.
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeOptions(command, rest, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	if n < len(args) {
		// Past the path: this is a positional position.
		return nil, cobra.ShellCompDirectiveNoFileComp
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

// awaitingOptionValue reports whether the word being completed is the
// pending value of a declared value option (space form "--name" as the
// last typed token).
func awaitingOptionValue(c config.Command, rest []string) bool {
	if len(rest) == 0 {
		return false
	}
	last := rest[len(rest)-1]
	if !strings.HasPrefix(last, "--") || strings.Contains(last, "=") {
		return false
	}
	for _, o := range c.Options {
		if o.Name == last[2:] {
			return !o.IsBool()
		}
	}
	return false
}

// completeOptions returns the command's declared options matching the
// typed prefix, in declaration order, with descriptions. Options
// already present among the typed tokens are skipped (repeats are
// legal but last-wins, so re-suggesting them is pointless). A --help
// entry is appended unless the command declares an option named help
// itself.
func completeOptions(c config.Command, rest []string, toComplete string) []string {
	used := func(name string) bool {
		for _, tok := range rest {
			if tok == "--"+name || strings.HasPrefix(tok, "--"+name+"=") {
				return true
			}
		}
		return false
	}
	var options []string
	for _, o := range c.Options {
		candidate := "--" + o.Name
		if !strings.HasPrefix(candidate, toComplete) || used(o.Name) {
			continue
		}
		if o.Description != "" {
			candidate += "\t" + o.Description
		}
		options = append(options, candidate)
	}
	if strings.HasPrefix("--help", toComplete) && !declaresOption(c, "help") {
		options = append(options, "--help\tshow this help")
	}
	return options
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

// configFile is one loaded command file and the directory its
// commands run in. Files are ordered by precedence (local before
// global); a command's origin file supplies its top-level env, shell,
// and working directory.
type configFile struct {
	cfg     *config.Config
	workDir string
}

// loadConfigFiles loads every located command file in precedence
// order. Any located file that fails to load or validate is a hard
// error — a broken global file surfaces everywhere rather than being
// silently ignored.
func loadConfigFiles() ([]configFile, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	found, err := config.Find(cwd)
	if err != nil {
		return nil, err
	}
	files := make([]configFile, 0, len(found))
	for _, f := range found {
		cfg, err := config.Load(f.Path)
		if err != nil {
			return nil, err
		}
		files = append(files, configFile{cfg: cfg, workDir: f.WorkDir})
	}
	return files, nil
}

// mergedCommands overlays the files' top-level commands, earlier
// (higher-precedence) files winning on name collisions: a local
// command shadows a same-named global one. Each subtree still comes
// entirely from one file, so walking the merged map resolves paths
// correctly.
func mergedCommands(files []configFile) map[string]config.Command {
	if len(files) == 1 {
		return files[0].cfg.Commands
	}
	merged := make(map[string]config.Command)
	for i := len(files) - 1; i >= 0; i-- {
		maps.Copy(merged, files[i].cfg.Commands)
	}
	return merged
}

// runCommand resolves the argument path through nested commands and
// executes the resolved command. A command without a run string lists
// its subcommands.
//
// Arguments stop being command path segments at the first "--", or at
// the first name that doesn't match a subcommand of a runnable command;
// the remainder is passed to the run string as positional parameters,
// after declared options are extracted from the pre-"--" portion. A
// --help among the pre-"--" remainder shows the command's declared
// help instead.
func runCommand(cmd *cobra.Command, args []string) error {
	files, err := loadConfigFiles()
	if err != nil {
		return err
	}

	// SetInterspersed(false) leaves a "--" after the command name in args,
	// so split it off here rather than via ArgsLenAtDash. Tokens after
	// the first "--" are always literal positionals, never options.
	path := args
	var literal []string
	explicit := false
	if i := slices.Index(args, "--"); i >= 0 {
		path, literal, explicit = args[:i], args[i+1:], true
	}
	if len(path) == 0 {
		return runList(cmd)
	}

	// The first path segment picks the origin file (local shadows
	// global); that file supplies the top-level env, shell, and the
	// working directory for everything below it.
	var origin configFile
	found := false
	for _, f := range files {
		if _, ok := f.cfg.Commands[path[0]]; ok {
			origin = f
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("command %q not found", path[0])
	}
	workDir := origin.workDir

	cmds := origin.cfg.Commands
	var command config.Command
	// Environment variables merge from outer to inner scopes, so
	// deeper definitions override same-named keys. Values stay
	// unevaluated until the command is known to execute, so overridden
	// dynamic entries never run. The shell inherits the same way: the
	// innermost declaration wins, empty means "sh".
	envVals := make(map[string]config.Value, len(origin.cfg.Env))
	maps.Copy(envVals, origin.cfg.Env)
	shell := origin.cfg.Shell
	n := 0
	for n < len(path) {
		c, ok := cmds[path[n]]
		if !ok {
			break
		}
		command = c
		maps.Copy(envVals, c.Env)
		if c.Shell != "" {
			shell = c.Shell
		}
		cmds = c.Commands
		n++
	}
	name := strings.Join(path[:n], " ")
	rest := path[n:] // tokens after the resolved path, before any "--"
	// A --help anywhere before "--" shows the command's declared help
	// instead of running it — checked before resolveEnv so help never
	// evaluates dynamic values. A declared option named "help" opts the
	// command out; "run cmd -- --help" passes a literal --help through.
	if slices.Contains(rest, "--help") && !declaresOption(command, "help") {
		return commandHelp(cmd.OutOrStdout(), command, name)
	}
	if len(rest) > 0 && command.Run == "" {
		return fmt.Errorf("command %q has no subcommand %q", name, rest[0])
	}

	if command.Run == "" {
		if len(literal) > 0 {
			return fmt.Errorf("command %q has no run", name)
		}
		return listCommands(cmd.OutOrStdout(), command.Commands, name)
	}

	env, err := resolveEnv(envVals, shell, workDir, name, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	// resolve evaluates a declared default. Dynamic defaults see the
	// fully resolved env, so they can reference shared values.
	resolve := func(v config.Value) (string, error) {
		if !v.IsDynamic() {
			return v.Literal, nil
		}
		return runner.Capture(shell, v.Run, workDir, envList(env), cmd.ErrOrStderr())
	}

	positional, optionArgs, optionEnv, err := applyOptions(command, name, rest, resolve)
	if err != nil {
		return err
	}
	// With an explicit "--", everything before it must be command path
	// or declared options.
	if explicit && len(positional) > 0 {
		return fmt.Errorf("command %q has no subcommand %q", name, positional[0])
	}
	// slices.Concat, not append: in the no-options passthrough case
	// positional aliases the args backing array, which literal follows.
	cmdArgs, argEnv, err := applyArguments(command, name, slices.Concat(positional, literal), resolve)
	if err != nil {
		return err
	}
	cmdArgs = append(cmdArgs, optionArgs...)
	maps.Copy(env, optionEnv)
	maps.Copy(env, argEnv) // declared arguments and options have the highest precedence
	return runner.Run(shell, command.Run, workDir, cmdArgs, envList(env), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
}

// applyArguments validates CLI arguments against the command's
// declared arguments, fills in defaults for missing trailing
// arguments, and builds an environment variable for each declared
// argument. Arguments beyond the declaration are passed through
// untouched. Defaults resolve lazily — a dynamic default only runs
// when it is actually used.
func applyArguments(command config.Command, name string, args []string, resolve func(config.Value) (string, error)) ([]string, map[string]string, error) {
	if len(command.Arguments) == 0 {
		return args, nil, nil
	}
	final := slices.Clone(args)
	env := make(map[string]string, len(command.Arguments))
	for i, decl := range command.Arguments {
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

// applyOptions extracts declared long-form options (--name,
// --name=value, --name value) from args, leaving everything else as
// positionals. It returns the positionals, the recognized options
// re-normalized as "--name value"/"--name" tokens in declaration order
// (appended after all positionals so $1..$n stay stable and "$@"
// forwards everything), and an environment variable per declared
// option: bools are "true"/"false", value options get the given value,
// their default, or "". Value options that are unset and have no
// default are omitted from the normalized tokens. Repeated options:
// the last one wins. Commands without an options declaration are
// passed through untouched. Defaults resolve lazily — a dynamic
// default only runs when it is actually used.
func applyOptions(command config.Command, name string, args []string, resolve func(config.Value) (string, error)) (positional, optionArgs []string, env map[string]string, err error) {
	if len(command.Options) == 0 {
		return args, nil, nil, nil
	}
	decls := make(map[string]config.Option, len(command.Options))
	for _, o := range command.Options {
		decls[o.Name] = o
	}
	bools := make(map[string]bool)
	values := make(map[string]string)
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if !strings.HasPrefix(tok, "--") {
			positional = append(positional, tok)
			continue
		}
		optName, value, hasValue := strings.Cut(tok[2:], "=")
		decl, ok := decls[optName]
		if !ok {
			return nil, nil, nil, fmt.Errorf("command %q: unknown option --%s", name, optName)
		}
		if decl.IsBool() {
			if hasValue {
				return nil, nil, nil, fmt.Errorf("command %q: option --%s does not take a value", name, optName)
			}
			bools[optName] = true
			continue
		}
		if !hasValue {
			i++
			if i >= len(args) {
				return nil, nil, nil, fmt.Errorf("command %q: option --%s requires a value", name, optName)
			}
			value = args[i] // taken literally, even if it looks like an option
		}
		values[optName] = value
	}

	env = make(map[string]string, len(command.Options))
	for _, decl := range command.Options {
		if decl.IsBool() {
			set := bools[decl.Name]
			env[decl.Name] = strconv.FormatBool(set)
			if set {
				optionArgs = append(optionArgs, "--"+decl.Name)
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
				return nil, nil, nil, fmt.Errorf("command %q: default for option --%s: %w", name, decl.Name, rerr)
			}
			value = v // defaults materialize into "$@" like argument defaults
		}
		env[decl.Name] = value
		optionArgs = append(optionArgs, "--"+decl.Name, value)
	}
	return positional, optionArgs, env, nil
}

// resolveEnv converts the merged env declarations to concrete strings,
// running dynamic values with the resolved shell in dir. A dynamic
// value sees the process environment plus the literal entries only —
// dynamic entries cannot reference one another, so no evaluation order
// is observable through the values; they still run in name order to
// keep any side effects deterministic.
func resolveEnv(env map[string]config.Value, shell, dir, name string, stderr io.Writer) (map[string]string, error) {
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
		out, err := runner.Capture(shell, env[k].Run, dir, literals, stderr)
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
