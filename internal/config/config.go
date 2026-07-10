package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents a task definition file. Env entries apply to
// every task in the file.
type Config struct {
	Env   map[string]string `yaml:"env"`
	Tasks map[string]Task   `yaml:"tasks"`
}

// Task represents a single task definition. A task may define a
// command, nested subtasks, or both. Subtasks may alternatively be
// loaded from an external task file referenced via file. Env entries
// apply to the task and its subtasks; inner definitions override
// same-named keys from outer scopes.
type Task struct {
	Description string            `yaml:"description"`
	Command     string            `yaml:"command"`
	File        string            `yaml:"file"`
	Env         map[string]string `yaml:"env"`
	Args        []Arg             `yaml:"args"`
	Tasks       map[string]Task   `yaml:"tasks"`
}

// Arg declares a named positional argument for a task's command.
// Default is a pointer to distinguish an absent default (required
// argument) from an explicit empty-string default.
type Arg struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Default     *string `yaml:"default"`
}

// Load reads and parses a task definition file, recursively expanding
// external task files referenced via file.
func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	cfg, err := loadFile(abs)
	if err != nil {
		return nil, err
	}

	if err := expandTasks(cfg.Tasks, filepath.Dir(abs), "", []string{abs}); err != nil {
		return nil, fmt.Errorf("invalid task file %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid task file %s: %w", path, err)
	}

	return cfg, nil
}

// loadFile reads and parses a single task file without expansion.
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

// expandTasks recursively replaces file references with the tasks
// defined in the referenced files. dir is the directory of the file
// the tasks came from, prefix is the space-joined task path for error
// messages, and chain holds the absolute paths of files currently
// being expanded, for cycle detection.
func expandTasks(tasks map[string]Task, dir, prefix string, chain []string) error {
	for name, task := range tasks {
		full := name
		if prefix != "" {
			full = prefix + " " + name
		}

		if task.File == "" {
			if err := expandTasks(task.Tasks, dir, full, chain); err != nil {
				return err
			}
			continue
		}

		if len(task.Tasks) > 0 {
			return fmt.Errorf("task %q: file and tasks are mutually exclusive", full)
		}

		ref := task.File
		if !filepath.IsAbs(ref) {
			ref = filepath.Join(dir, ref)
		}
		ref = filepath.Clean(ref)

		if slices.Contains(chain, ref) {
			return fmt.Errorf("circular task file reference: %s -> %s", strings.Join(chain, " -> "), ref)
		}

		sub, err := loadFile(ref)
		if err != nil {
			return fmt.Errorf("task %q: %w", full, err)
		}
		if len(sub.Tasks) == 0 {
			return fmt.Errorf("task %q: no tasks defined in %s", full, ref)
		}

		if err := expandTasks(sub.Tasks, filepath.Dir(ref), full, append(chain, ref)); err != nil {
			return err
		}

		// The external file's top-level env applies to all tasks it
		// defines; being closer to those tasks, it wins over the
		// referencing task's env on conflict.
		if len(sub.Env) > 0 {
			if task.Env == nil {
				task.Env = make(map[string]string, len(sub.Env))
			}
			maps.Copy(task.Env, sub.Env)
		}
		task.Tasks = sub.Tasks
		task.File = ""
		tasks[name] = task
	}
	return nil
}

// Validate checks that the config is well-formed.
func (c *Config) Validate() error {
	if len(c.Tasks) == 0 {
		return fmt.Errorf("no tasks defined")
	}
	if err := validateEnv(c.Env, ""); err != nil {
		return err
	}
	return validateTasks(c.Tasks, "")
}

func validateTasks(tasks map[string]Task, prefix string) error {
	for name, task := range tasks {
		full := name
		if prefix != "" {
			full = prefix + " " + name
		}
		if task.Command == "" && len(task.Tasks) == 0 {
			return fmt.Errorf("task %q has no command or subtasks", full)
		}
		if err := validateEnv(task.Env, full); err != nil {
			return err
		}
		if err := validateArgs(task, full); err != nil {
			return err
		}
		if err := validateTasks(task.Tasks, full); err != nil {
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
		scope = fmt.Sprintf("task %q", full)
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

// validateArgs checks a task's args declaration. Args map CLI
// arguments positionally, so an argument without a default may not
// follow one with a default: it could never be filled without also
// overriding the earlier default.
func validateArgs(task Task, full string) error {
	if len(task.Args) == 0 {
		return nil
	}
	if task.Command == "" {
		return fmt.Errorf("task %q declares args but has no command", full)
	}
	seen := make(map[string]bool, len(task.Args))
	sawDefault := false
	for _, arg := range task.Args {
		if arg.Name == "" {
			return fmt.Errorf("task %q has an argument without a name", full)
		}
		if seen[arg.Name] {
			return fmt.Errorf("task %q has duplicate argument %q", full, arg.Name)
		}
		seen[arg.Name] = true
		if arg.Default != nil {
			sawDefault = true
		} else if sawDefault {
			return fmt.Errorf("task %q: required argument %q may not follow an argument with a default", full, arg.Name)
		}
	}
	return nil
}
