package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents a command definition file. Env entries apply to
// every command in the file. Includes name further command files whose
// commands are merged into the top level.
type Config struct {
	Env      map[string]string  `yaml:"env"`
	Includes []string           `yaml:"includes"`
	Commands map[string]Command `yaml:"commands"`
}

// Command represents a single command definition. A command may define
// a run string, nested subcommands, or both. Includes name external
// command files whose commands are merged into this command's
// subcommands. Env entries apply to the command and its subcommands;
// inner definitions override same-named keys from outer scopes.
type Command struct {
	Description string             `yaml:"description"`
	Run         string             `yaml:"run"`
	Includes    []string           `yaml:"includes"`
	Env         map[string]string  `yaml:"env"`
	Args        []Arg              `yaml:"args"`
	Flags       []Flag             `yaml:"flags"`
	Commands    map[string]Command `yaml:"commands"`
}

// Arg declares a named positional argument for a command's run string.
// Default is a pointer to distinguish an absent default (required
// argument) from an explicit empty-string default.
type Arg struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Default     *string `yaml:"default"`
}

// Flag declares a named long-form option (--name) for a command's run
// string. Type selects between a value option ("" or "string") and a
// boolean flag ("bool"). Default is a pointer to distinguish an absent
// default from an explicit empty-string default; it is only valid for
// value options.
type Flag struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Type        string  `yaml:"type"`
	Default     *string `yaml:"default"`
}

// IsBool reports whether the flag is a boolean flag rather than a
// value option.
func (f Flag) IsBool() bool { return f.Type == "bool" }

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
func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	return &cfg, nil
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
					c.Env = make(map[string]string, len(sub.Env))
				}
				c.Env[k] = v
			}
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

// Validate checks that the config is well-formed.
func (c *Config) Validate() error {
	if len(c.Commands) == 0 {
		return fmt.Errorf("no commands defined")
	}
	// "self" is the only reserved name: it holds run's own built-in
	// subcommands (list, version, completion). Nested commands may
	// still use the name freely.
	if _, ok := c.Commands["self"]; ok {
		return fmt.Errorf("command name %q is reserved for run's built-in commands", "self")
	}
	if err := validateEnv(c.Env, ""); err != nil {
		return err
	}
	return validateCommands(c.Commands, "")
}

func validateCommands(cmds map[string]Command, prefix string) error {
	for name, c := range cmds {
		full := name
		if prefix != "" {
			full = prefix + " " + name
		}
		if c.Run == "" && len(c.Commands) == 0 {
			return fmt.Errorf("command %q has no run or subcommands", full)
		}
		if err := validateEnv(c.Env, full); err != nil {
			return err
		}
		if err := validateArgs(c, full); err != nil {
			return err
		}
		if err := validateFlags(c, full); err != nil {
			return err
		}
		if err := validateCommands(c.Commands, full); err != nil {
			return err
		}
	}
	return nil
}

// validateEnv checks environment variable names. Values are literal
// strings and may be anything, including empty.
func validateEnv(env map[string]string, full string) error {
	scope := "top-level env"
	if full != "" {
		scope = fmt.Sprintf("command %q", full)
	}
	for name := range env {
		if name == "" {
			return fmt.Errorf("%s has an environment variable without a name", scope)
		}
		if strings.Contains(name, "=") {
			return fmt.Errorf("%s: environment variable name %q must not contain '='", scope, name)
		}
	}
	return nil
}

// validateArgs checks a command's args declaration. Args map CLI
// arguments positionally, so an argument without a default may not
// follow one with a default: it could never be filled without also
// overriding the earlier default.
func validateArgs(c Command, full string) error {
	if len(c.Args) == 0 {
		return nil
	}
	if c.Run == "" {
		return fmt.Errorf("command %q declares args but has no run", full)
	}
	seen := make(map[string]bool, len(c.Args))
	sawDefault := false
	for _, arg := range c.Args {
		if arg.Name == "" {
			return fmt.Errorf("command %q has an argument without a name", full)
		}
		if seen[arg.Name] {
			return fmt.Errorf("command %q has duplicate argument %q", full, arg.Name)
		}
		seen[arg.Name] = true
		if arg.Default != nil {
			sawDefault = true
		} else if sawDefault {
			return fmt.Errorf("command %q: required argument %q may not follow an argument with a default", full, arg.Name)
		}
	}
	return nil
}

// validateFlags checks a command's flags declaration. Flags are
// matched on the CLI as "--name" and exported as environment
// variables, so names must be parseable as both. A flag may not share
// a name with a declared argument: both become environment variables,
// so the value would be ambiguous. Bool flags may not declare a
// default: without a --no-name form a true default could never be
// turned off, so unset always means false.
func validateFlags(c Command, full string) error {
	if len(c.Flags) == 0 {
		return nil
	}
	if c.Run == "" {
		return fmt.Errorf("command %q declares flags but has no run", full)
	}
	argNames := make(map[string]bool, len(c.Args))
	for _, arg := range c.Args {
		argNames[arg.Name] = true
	}
	seen := make(map[string]bool, len(c.Flags))
	for _, f := range c.Flags {
		if f.Name == "" {
			return fmt.Errorf("command %q has a flag without a name", full)
		}
		if strings.Contains(f.Name, "=") {
			return fmt.Errorf("command %q: flag name %q must not contain '='", full, f.Name)
		}
		if strings.HasPrefix(f.Name, "-") {
			return fmt.Errorf("command %q: flag name %q must not start with '-'", full, f.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("command %q has duplicate flag %q", full, f.Name)
		}
		seen[f.Name] = true
		if argNames[f.Name] {
			return fmt.Errorf("command %q: flag %q collides with an argument of the same name", full, f.Name)
		}
		switch f.Type {
		case "", "string", "bool":
		default:
			return fmt.Errorf("command %q: flag %q has invalid type %q (supported: string, bool)", full, f.Name, f.Type)
		}
		if f.IsBool() && f.Default != nil {
			return fmt.Errorf("command %q: bool flag %q may not have a default", full, f.Name)
		}
	}
	return nil
}
