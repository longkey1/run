package config

import (
	"fmt"
	"os"
	"path/filepath"
)

var localNames = []string{".run.yaml", ".run.yml"}

var globalNames = []string{"run.yaml", "run.yml"}

// Source is one located command file and the directory its commands
// run in.
type Source struct {
	Path    string
	WorkDir string
}

// Find locates command definition files and returns them in precedence
// order (highest first), each with the working directory its commands
// should run in.
//
// Search order:
//  1. $RUN_CONFIG (commands run in the current directory); used alone,
//     nothing else is merged
//  2. .run.yaml / .run.yml in cwd or any ancestor directory
//     (commands run in the directory containing the file)
//  3. ~/.config/run/run.yaml / run.yml (commands run in the current
//     directory)
//
// Without $RUN_CONFIG, the local and global files are both returned
// when both exist, local first: their commands are merged with local
// definitions shadowing same-named global top-level commands.
func Find(cwd string) ([]Source, error) {
	if env := os.Getenv("RUN_CONFIG"); env != "" {
		if _, err := os.Stat(env); err != nil {
			return nil, fmt.Errorf("RUN_CONFIG points to an unreadable file: %w", err)
		}
		return []Source{{Path: env, WorkDir: cwd}}, nil
	}

	var sources []Source

	dir := cwd
loop:
	for {
		for _, name := range localNames {
			candidate := filepath.Join(dir, name)
			if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
				sources = append(sources, Source{Path: candidate, WorkDir: dir})
				break loop
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
		// Without a home directory there is no global file to merge;
		// only fail if nothing else was found either.
		if len(sources) > 0 {
			return sources, nil
		}
		return nil, err
	}
	globalDir := filepath.Join(home, ".config", "run")
	for _, name := range globalNames {
		candidate := filepath.Join(globalDir, name)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			sources = append(sources, Source{Path: candidate, WorkDir: cwd})
			break
		}
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no command file found (.run.yaml or %s)", filepath.Join(globalDir, "run.yaml"))
	}
	return sources, nil
}
