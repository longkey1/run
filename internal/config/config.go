package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents a task definition file.
type Config struct {
	Tasks map[string]Task `yaml:"tasks"`
}

// Task represents a single task definition. A task may define a
// command, nested subtasks, or both.
type Task struct {
	Description string          `yaml:"description"`
	Command     string          `yaml:"command"`
	Tasks       map[string]Task `yaml:"tasks"`
}

// Load reads and parses a task definition file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid task file %s: %w", path, err)
	}

	return &cfg, nil
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
