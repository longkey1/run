package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents a task definition file.
type Config struct {
	Tasks map[string]Task `yaml:"tasks"`
}

// Task represents a single task definition. A task may define a
// command, nested subtasks, or both. Subtasks may alternatively be
// loaded from an external task file referenced via file.
type Task struct {
	Description string          `yaml:"description"`
	Command     string          `yaml:"command"`
	File        string          `yaml:"file"`
	Tasks       map[string]Task `yaml:"tasks"`
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
		if err := validateTasks(task.Tasks, full); err != nil {
			return err
		}
	}
	return nil
}
