package config

import (
	"fmt"
	"os"
	"path/filepath"
)

var localNames = []string{".run.yaml", ".run.yml"}

var globalNames = []string{"run.yaml", "run.yml"}

// Find locates a task definition file and returns its path along with the
// working directory tasks should run in.
//
// Search order:
//  1. $RUN_CONFIG (tasks run in the current directory)
//  2. .run.yaml / .run.yml in cwd or any ancestor directory
//     (tasks run in the directory containing the file)
//  3. ~/.config/run/run.yaml / run.yml (tasks run in the current directory)
func Find(cwd string) (path string, workDir string, err error) {
	if env := os.Getenv("RUN_CONFIG"); env != "" {
		if _, err := os.Stat(env); err != nil {
			return "", "", fmt.Errorf("RUN_CONFIG points to an unreadable file: %w", err)
		}
		return env, cwd, nil
	}

	dir := cwd
	for {
		for _, name := range localNames {
			candidate := filepath.Join(dir, name)
			if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
				return candidate, dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	globalDir := filepath.Join(home, ".config", "run")
	for _, name := range globalNames {
		candidate := filepath.Join(globalDir, name)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate, cwd, nil
		}
	}

	return "", "", fmt.Errorf("no task file found (.run.yaml or %s)", filepath.Join(globalDir, "run.yaml"))
}
