package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/longkey1/run/internal/config"
	"github.com/spf13/cobra"
)

func runList(cmd *cobra.Command) error {
	files, err := loadConfigFiles()
	if err != nil {
		return err
	}
	return listCommands(cmd.OutOrStdout(), mergedCommands(files), "")
}

// listCommands writes all runnable commands (those with a run string)
// under the given prefix, one per line with the full space-joined path.
func listCommands(out io.Writer, cmds map[string]config.Command, prefix string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	writeCommands(w, cmds, prefix)
	return w.Flush()
}

func writeCommands(w io.Writer, cmds map[string]config.Command, prefix string) {
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		c := cmds[name]
		full := name
		if prefix != "" {
			full = prefix + " " + name
		}
		if c.Run != "" {
			label := full + argumentSignature(c.Arguments) + optionSignature(c.Options)
			if c.Description == "" {
				_, _ = fmt.Fprintf(w, "  %s\n", label)
			} else {
				_, _ = fmt.Fprintf(w, "  %s\t- %s\n", label, c.Description)
			}
		}
		writeCommands(w, c.Commands, full)
	}
}

// argumentSignature renders declared arguments as " <name>" for
// required arguments and " [name]" for arguments with a default.
func argumentSignature(args []config.Argument) string {
	var b strings.Builder
	for _, a := range args {
		if a.Default != nil {
			fmt.Fprintf(&b, " [%s]", a.Name)
		} else {
			fmt.Fprintf(&b, " <%s>", a.Name)
		}
	}
	return b.String()
}

// optionSignature renders declared options after the argument
// signature. All options are optional, so every entry is bracketed:
// " [--name]" for bool options, " [--name <name>]" for value options.
func optionSignature(options []config.Option) string {
	var b strings.Builder
	for _, o := range options {
		if o.IsBool() {
			fmt.Fprintf(&b, " [--%s]", o.Name)
		} else {
			fmt.Fprintf(&b, " [--%s <%s>]", o.Name, o.Name)
		}
	}
	return b.String()
}

// commandJSON is one command in `run self list --json` output: the
// declaration plus its effective execution context (shell, env,
// isolation, working directory, origin file), resolved the same way
// runCommand resolves it. Groups (no run string) are included with
// "run" omitted. Dynamic values appear as {"run": ...} — never
// evaluated, like --help.
type commandJSON struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Run         string                  `json:"run,omitempty"`
	Shell       string                  `json:"shell"`
	WorkDir     string                  `json:"workdir"`
	Source      string                  `json:"source"`
	InheritEnv  bool                    `json:"inherit_env"`
	PassEnv     []string                `json:"pass_env,omitempty"`
	Env         map[string]config.Value `json:"env,omitempty"`
	Arguments   []argumentJSON          `json:"arguments,omitempty"`
	Options     []optionJSON            `json:"options,omitempty"`
}

type argumentJSON struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Required    bool          `json:"required"`
	Default     *config.Value `json:"default,omitempty"`
}

type optionJSON struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Type        string        `json:"type"`
	Default     *config.Value `json:"default,omitempty"`
}

func runListJSON(cmd *cobra.Command) error {
	files, err := loadConfigFiles()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(jsonCommands(files))
}

// jsonScope carries the execution context accumulated along a command
// path: the origin file's constants (workDir, source) and the settings
// that resolve along the walk (shell, isolation, env).
type jsonScope struct {
	shell      string
	inheritEnv *bool
	passEnv    []string
	env        map[string]config.Value
	workDir    string
	source     string
}

// jsonCommands flattens the merged command tree into one entry per
// command, sorted by path. Top-level shadowing picks the origin file
// per name exactly like runCommand: the whole subtree carries that
// file's top-level settings, working directory, and source path.
func jsonCommands(files []configFile) []commandJSON {
	entries := []commandJSON{}
	seen := make(map[string]bool)
	for _, f := range files {
		source := f.path
		if abs, err := filepath.Abs(f.path); err == nil {
			source = abs
		}
		scope := jsonScope{
			shell:      f.cfg.Shell,
			inheritEnv: f.cfg.InheritEnv,
			passEnv:    f.cfg.PassEnv,
			env:        f.cfg.Env,
			workDir:    f.workDir,
			source:     source,
		}
		for name, c := range f.cfg.Commands {
			if seen[name] {
				continue
			}
			seen[name] = true
			entries = appendCommandJSON(entries, name, c, scope)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

// appendCommandJSON appends the entry for one command and recurses
// into its subcommands. The scope is passed by value: each level's
// overrides stay local to its subtree, mirroring runCommand's walk
// (innermost shell/inherit_env win, pass_env accumulates, inner env
// keys override outer ones). The "sh" default materializes here so
// consumers need no knowledge of it.
func appendCommandJSON(entries []commandJSON, name string, c config.Command, s jsonScope) []commandJSON {
	if c.Shell != "" {
		s.shell = c.Shell
	}
	if c.InheritEnv != nil {
		s.inheritEnv = c.InheritEnv
	}
	s.passEnv = slices.Concat(s.passEnv, c.PassEnv)
	if len(c.Env) > 0 {
		merged := make(map[string]config.Value, len(s.env)+len(c.Env))
		maps.Copy(merged, s.env)
		maps.Copy(merged, c.Env)
		s.env = merged
	}

	shell := s.shell
	if shell == "" {
		shell = "sh"
	}
	entries = append(entries, commandJSON{
		Name:        name,
		Description: c.Description,
		Run:         c.Run,
		Shell:       shell,
		WorkDir:     s.workDir,
		Source:      s.source,
		InheritEnv:  s.inheritEnv == nil || *s.inheritEnv,
		PassEnv:     s.passEnv,
		Env:         s.env,
		Arguments:   argumentsJSON(c.Arguments),
		Options:     optionsJSON(c.Options),
	})
	for _, sub := range slices.Sorted(maps.Keys(c.Commands)) {
		entries = appendCommandJSON(entries, name+" "+sub, c.Commands[sub], s)
	}
	return entries
}

func argumentsJSON(args []config.Argument) []argumentJSON {
	if len(args) == 0 {
		return nil
	}
	out := make([]argumentJSON, len(args))
	for i, a := range args {
		out[i] = argumentJSON{Name: a.Name, Description: a.Description, Required: a.Default == nil, Default: a.Default}
	}
	return out
}

// optionsJSON renders declared options with the "string" default type
// materialized, so consumers see only "string" or "bool".
func optionsJSON(options []config.Option) []optionJSON {
	if len(options) == 0 {
		return nil
	}
	out := make([]optionJSON, len(options))
	for i, o := range options {
		typ := o.Type
		if typ == "" {
			typ = "string"
		}
		out[i] = optionJSON{Name: o.Name, Description: o.Description, Type: typ, Default: o.Default}
	}
	return out
}
