package config

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents a command definition file. Env entries apply to
// every command in the file. Shell names the shell that executes run
// strings and dynamic values (empty means "sh"). InheritEnv and
// PassEnv control environment isolation for the file's commands: with
// inherit_env: false a command receives only a fixed baseline of the
// process environment plus the names matching a pass_env pattern
// (InheritEnv is a pointer so an inner scope can override either way).
// Includes name further command files whose commands are merged into
// the top level. Extensions absorbs the remaining top-level keys,
// which KnownFields would otherwise reject; loadFile then allows x-*
// extension keys (never read — a place to define YAML anchors shared
// across commands in the file, resolved by the parser) and rejects
// everything else.
type Config struct {
	Shell      string               `yaml:"shell"`
	Env        map[string]Value     `yaml:"env"`
	InheritEnv *bool                `yaml:"inherit_env"`
	PassEnv    []string             `yaml:"pass_env"`
	Includes   []string             `yaml:"includes"`
	Commands   map[string]Command   `yaml:"commands"`
	Extensions map[string]yaml.Node `yaml:",inline"`
}

// Value is a string setting that is either literal or dynamic. The
// dynamic form ({run: ...}) names a shell command whose stdout (with
// trailing newlines trimmed, like $(...) substitution) becomes the
// value when the resolved command is executed.
type Value struct {
	Literal string
	Run     string
}

// IsDynamic reports whether the value is computed by a shell command
// rather than taken literally.
func (v Value) IsDynamic() bool { return v.Run != "" }

// MarshalJSON mirrors the YAML source forms: a literal value is a
// plain string, a dynamic value is {"run": ...} — never evaluated.
func (v Value) MarshalJSON() ([]byte, error) {
	if v.IsDynamic() {
		return json.Marshal(struct {
			Run string `json:"run"`
		}{v.Run})
	}
	return json.Marshal(v.Literal)
}

// UnmarshalYAML accepts a plain scalar (literal value) or a mapping
// with a single non-empty "run" key (dynamic value).
func (v *Value) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Decode(&v.Literal)
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			if key := node.Content[i].Value; key != "run" {
				return fmt.Errorf("line %d: unknown key %q in dynamic value (only \"run\" is allowed)", node.Content[i].Line, key)
			}
		}
		var m struct {
			Run string `yaml:"run"`
		}
		if err := node.Decode(&m); err != nil {
			return err
		}
		if m.Run == "" {
			return fmt.Errorf("line %d: dynamic value must have a non-empty run", node.Line)
		}
		v.Run = m.Run
		return nil
	default:
		return fmt.Errorf("line %d: value must be a string or {run: ...}", node.Line)
	}
}

// Command represents a single command definition. A command may define
// a run string, nested subcommands, or both. Includes name external
// command files whose commands are merged into this command's
// subcommands. Env entries apply to the command and its subcommands;
// inner definitions override same-named keys from outer scopes. Shell
// overrides the shell for the command and its subcommands, like env.
// InheritEnv overrides environment isolation for the command and its
// subcommands (innermost declaration wins); PassEnv patterns
// accumulate along the command path.
type Command struct {
	Description string             `yaml:"description"`
	Run         string             `yaml:"run"`
	Shell       string             `yaml:"shell"`
	InheritEnv  *bool              `yaml:"inherit_env"`
	PassEnv     []string           `yaml:"pass_env"`
	Includes    []string           `yaml:"includes"`
	Env         map[string]Value   `yaml:"env"`
	Arguments   []Argument         `yaml:"arguments"`
	Options     []Option           `yaml:"options"`
	Commands    map[string]Command `yaml:"commands"`
}

// Argument declares a named positional argument for a command's run
// string. Default is a pointer to distinguish an absent default
// (required argument) from an explicit empty-string default.
type Argument struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Default     *Value `yaml:"default"`
}

// Option declares a named long-form option (--name) for a command's
// run string. Type selects between a value option ("" or "string") and
// a boolean option ("bool"). Default is a pointer to distinguish an
// absent default from an explicit empty-string default; it is only
// valid for value options.
type Option struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
	Default     *Value `yaml:"default"`
}

// IsBool reports whether the option is a boolean switch rather than a
// value option.
func (o Option) IsBool() bool { return o.Type == "bool" }

// Load reads and parses a command definition file, recursively merging
// external command files referenced via includes.
func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	cfg, err := loadFile(abs)
	if err != nil {
		return nil, err
	}

	cmds, err := expandIncludes(cfg.Commands, cfg.Includes, filepath.Dir(abs), "", []string{abs})
	if err != nil {
		return nil, fmt.Errorf("invalid command file %s: %w", path, err)
	}
	cfg.Commands = cmds
	cfg.Includes = nil

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid command file %s: %w", path, err)
	}

	return cfg, nil
}

// loadFile reads and parses a single command file without expansion.
// Unknown keys are a parse error, so a misspelled key (e.g. argments:)
// fails loudly instead of silently dropping the declaration — except
// top-level x-* extension keys, which are ignored. An empty file
// decodes to an empty config; Validate rejects it later with a clearer
// error than the decoder's io.EOF.
func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if err := checkExtensionKeys(cfg.Extensions, data); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	return &cfg, nil
}

// checkExtensionKeys rejects unknown top-level keys absorbed by the
// inline Extensions map during strict decoding: only x-* keys are
// allowed. The map holds value nodes, whose line differs from the
// key's for block values, so the document is re-parsed (only on this
// error path) to report the exact key lines.
func checkExtensionKeys(ext map[string]yaml.Node, data []byte) error {
	var bad []string
	for key := range ext {
		if !strings.HasPrefix(key, "x-") {
			bad = append(bad, key)
		}
	}
	if len(bad) == 0 {
		return nil
	}

	lines := topLevelKeyLines(data)
	slices.SortFunc(bad, func(a, b string) int { return cmp.Compare(lines[a], lines[b]) })
	msgs := make([]string, len(bad))
	for i, key := range bad {
		msgs[i] = fmt.Sprintf("line %d: unknown key %q (top-level keys outside the schema must start with \"x-\")", lines[key], key)
	}
	return fmt.Errorf("yaml: unmarshal errors:\n  %s", strings.Join(msgs, "\n  "))
}

// topLevelKeyLines returns the line number of each top-level mapping
// key in the document.
func topLevelKeyLines(data []byte) map[string]int {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return nil
	}
	lines := make(map[string]int, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		lines[m.Content[i].Value] = m.Content[i].Line
	}
	return lines
}

// expandIncludes recursively merges included command files into cmds.
// Commands from an included file land flat in the including scope; a
// name collision with a local command or an earlier include is an
// error. dir is the directory of the file the commands came from,
// prefix is the space-joined command path for error messages, and
// chain holds the absolute paths of files currently being expanded,
// for cycle detection.
func expandIncludes(cmds map[string]Command, includes []string, dir, prefix string, chain []string) (map[string]Command, error) {
	for name, c := range cmds {
		full := name
		if prefix != "" {
			full = prefix + " " + name
		}
		sub, err := expandIncludes(c.Commands, c.Includes, dir, full, chain)
		if err != nil {
			return nil, err
		}
		c.Commands = sub
		c.Includes = nil
		cmds[name] = c
	}

	if len(includes) > 0 && cmds == nil {
		cmds = make(map[string]Command)
	}
	for _, ref := range includes {
		if !filepath.IsAbs(ref) {
			ref = filepath.Join(dir, ref)
		}
		ref = filepath.Clean(ref)

		if slices.Contains(chain, ref) {
			return nil, fmt.Errorf("circular include: %s -> %s", strings.Join(chain, " -> "), ref)
		}

		sub, err := loadFile(ref)
		if err != nil {
			return nil, includeErr(prefix, err)
		}

		subCmds, err := expandIncludes(sub.Commands, sub.Includes, filepath.Dir(ref), prefix, append(chain, ref))
		if err != nil {
			return nil, err
		}
		if len(subCmds) == 0 {
			return nil, includeErr(prefix, fmt.Errorf("no commands defined in %s", ref))
		}

		for name, c := range subCmds {
			if _, ok := cmds[name]; ok {
				return nil, includeErr(prefix, fmt.Errorf("include %s: command %q already defined", ref, name))
			}
			// The included file's top-level env applies to every command
			// it defines; being closer to those commands, it wins over
			// outer scopes but not over the command's own env.
			for k, v := range sub.Env {
				if _, ok := c.Env[k]; ok {
					continue
				}
				if c.Env == nil {
					c.Env = make(map[string]Value, len(sub.Env))
				}
				c.Env[k] = v
			}
			// Same for the included file's top-level shell.
			if c.Shell == "" {
				c.Shell = sub.Shell
			}
			// The included file's top-level isolation settings push down
			// the same way: the command's own inherit_env wins, and its
			// pass_env patterns are joined with the file's (patterns are
			// order-insensitive, so a plain union suffices).
			if c.InheritEnv == nil {
				c.InheritEnv = sub.InheritEnv
			}
			c.PassEnv = append(c.PassEnv, sub.PassEnv...)
			cmds[name] = c
		}
	}
	return cmds, nil
}

// includeErr scopes an include-related error to the command path it
// occurred under, if any.
func includeErr(prefix string, err error) error {
	if prefix == "" {
		return err
	}
	return fmt.Errorf("command %q: %w", prefix, err)
}

// Validate checks that the config is well-formed. Every violation is
// collected and returned joined (one per line via errors.Join) rather
// than stopping at the first, so a single edit-validate cycle surfaces
// the full list. Commands and env keys are checked in sorted order so
// the report is deterministic.
func (c *Config) Validate() error {
	var errs []error
	if len(c.Commands) == 0 {
		errs = append(errs, errors.New("no commands defined"))
	}
	// "self" is the only reserved name: it holds run's own built-in
	// subcommands (list, version, completion). Nested commands may
	// still use the name freely.
	if _, ok := c.Commands["self"]; ok {
		errs = append(errs, fmt.Errorf("command name %q is reserved for run's built-in commands", "self"))
	}
	errs = append(errs, validateEnv(c.Env, "")...)
	errs = append(errs, validatePassEnv(c.PassEnv, "")...)
	errs = append(errs, validateCommands(c.Commands, "")...)
	return errors.Join(errs...)
}

func validateCommands(cmds map[string]Command, prefix string) []error {
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(cmds)) {
		c := cmds[name]
		full := name
		if prefix != "" {
			full = prefix + " " + name
		}
		if c.Run == "" && len(c.Commands) == 0 {
			errs = append(errs, fmt.Errorf("command %q has no run or subcommands", full))
		}
		errs = append(errs, validateEnv(c.Env, full)...)
		errs = append(errs, validatePassEnv(c.PassEnv, full)...)
		errs = append(errs, validateArguments(c, full)...)
		errs = append(errs, validateOptions(c, full)...)
		errs = append(errs, validateCommands(c.Commands, full)...)
	}
	return errs
}

// validateEnv checks environment variable names. Literal values may
// be anything, including empty; the shape of dynamic values is
// enforced at unmarshal time.
func validateEnv(env map[string]Value, full string) []error {
	scope := "top-level env"
	if full != "" {
		scope = fmt.Sprintf("command %q", full)
	}
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(env)) {
		if name == "" {
			errs = append(errs, fmt.Errorf("%s has an environment variable without a name", scope))
			continue
		}
		if strings.Contains(name, "=") {
			errs = append(errs, fmt.Errorf("%s: environment variable name %q must not contain '='", scope, name))
		}
	}
	return errs
}

// validatePassEnv checks pass_env patterns. Entries are matched
// against environment variable names with path.Match, so glob
// metacharacters (*, ?, [...]) are allowed; malformed patterns are
// rejected here so they can't silently match nothing at execution
// time.
func validatePassEnv(passEnv []string, full string) []error {
	scope := "top-level pass_env"
	if full != "" {
		scope = fmt.Sprintf("command %q", full)
	}
	var errs []error
	for _, p := range passEnv {
		if p == "" {
			errs = append(errs, fmt.Errorf("%s has an empty pass_env entry", scope))
			continue
		}
		if strings.Contains(p, "=") {
			errs = append(errs, fmt.Errorf("%s: pass_env entry %q must not contain '='", scope, p))
		}
		if _, err := path.Match(p, ""); err != nil {
			errs = append(errs, fmt.Errorf("%s: pass_env entry %q is not a valid pattern", scope, p))
		}
	}
	return errs
}

// validateArguments checks a command's arguments declaration.
// Arguments map CLI arguments positionally, so an argument without a
// default may not follow one with a default: it could never be filled
// without also overriding the earlier default.
func validateArguments(c Command, full string) []error {
	if len(c.Arguments) == 0 {
		return nil
	}
	var errs []error
	if c.Run == "" {
		errs = append(errs, fmt.Errorf("command %q declares arguments but has no run", full))
	}
	seen := make(map[string]bool, len(c.Arguments))
	sawDefault := false
	for _, arg := range c.Arguments {
		if arg.Default != nil {
			sawDefault = true
		}
		if arg.Name == "" {
			errs = append(errs, fmt.Errorf("command %q has an argument without a name", full))
			continue
		}
		if seen[arg.Name] {
			errs = append(errs, fmt.Errorf("command %q has duplicate argument %q", full, arg.Name))
		}
		seen[arg.Name] = true
		if arg.Default == nil && sawDefault {
			errs = append(errs, fmt.Errorf("command %q: required argument %q may not follow an argument with a default", full, arg.Name))
		}
	}
	return errs
}

// validateOptions checks a command's options declaration. Options are
// matched on the CLI as "--name" and exported as environment
// variables, so names must be parseable as both. An option may not
// share a name with a declared argument: both become environment
// variables, so the value would be ambiguous. Bool options may not
// declare a default: without a --no-name form a true default could
// never be turned off, so unset always means false.
func validateOptions(c Command, full string) []error {
	if len(c.Options) == 0 {
		return nil
	}
	var errs []error
	if c.Run == "" {
		errs = append(errs, fmt.Errorf("command %q declares options but has no run", full))
	}
	argNames := make(map[string]bool, len(c.Arguments))
	for _, arg := range c.Arguments {
		argNames[arg.Name] = true
	}
	seen := make(map[string]bool, len(c.Options))
	for _, o := range c.Options {
		if o.Name == "" {
			errs = append(errs, fmt.Errorf("command %q has an option without a name", full))
			continue
		}
		if strings.Contains(o.Name, "=") {
			errs = append(errs, fmt.Errorf("command %q: option name %q must not contain '='", full, o.Name))
		}
		if strings.HasPrefix(o.Name, "-") {
			errs = append(errs, fmt.Errorf("command %q: option name %q must not start with '-'", full, o.Name))
		}
		if seen[o.Name] {
			errs = append(errs, fmt.Errorf("command %q has duplicate option %q", full, o.Name))
		}
		seen[o.Name] = true
		if argNames[o.Name] {
			errs = append(errs, fmt.Errorf("command %q: option %q collides with an argument of the same name", full, o.Name))
		}
		switch o.Type {
		case "", "string", "bool":
		default:
			errs = append(errs, fmt.Errorf("command %q: option %q has invalid type %q (supported: string, bool)", full, o.Name, o.Type))
		}
		if o.IsBool() && o.Default != nil {
			errs = append(errs, fmt.Errorf("command %q: bool option %q may not have a default", full, o.Name))
		}
	}
	return errs
}
