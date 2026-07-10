package config

import (
	"fmt"
	"os"
	"path/filepath"
)

var localNames = []string{".run.yaml", ".run.yml"}

// localDirNames is the directory form of the local command file: the
// entry file inside a .run directory. Commands still run in the
// directory containing .run, not in .run itself; includes paths
// inside the file resolve against .run as usual.
var localDirNames = []string{
	filepath.Join(".run", "run.yaml"),
	filepath.Join(".run", "run.yml"),
}

var globalNames = []string{"run.yaml", "run.yml"}

// File is one located command file and the directory its commands
// run in. Global marks the global file (~/.config/run); the local
// file and a $RUN_CONFIG file are not global.
type File struct {
	Path    string
	WorkDir string
	Global  bool
}

// GlobalDir returns the global config directory (~/.config/run). The
// directory is a fixed location derived from the home directory; it
// is returned whether or not it (or a command file inside it) exists.
func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "run"), nil
}

// Find locates command definition files and returns them in precedence
// order (highest first), each with the working directory its commands
// should run in.
//
// Search order:
//  1. $RUN_CONFIG (commands run in the current directory); used alone,
//     nothing else is merged
//  2. .run.yaml / .run.yml or .run/run.yaml / .run/run.yml in cwd or
//     any ancestor directory (commands run in that directory — for the
//     .run form, the directory containing .run). Both forms in the
//     same directory is an error
//  3. ~/.config/run/run.yaml / run.yml (commands run in the current
//     directory)
//
// Without $RUN_CONFIG, the local and global files are both returned
// when both exist, local first: their commands are merged with local
// definitions shadowing same-named global top-level commands.
func Find(cwd string) ([]File, error) {
	if env := os.Getenv("RUN_CONFIG"); env != "" {
		if _, err := os.Stat(env); err != nil {
			return nil, fmt.Errorf("RUN_CONFIG points to an unreadable file: %w", err)
		}
		return []File{{Path: env, WorkDir: cwd}}, nil
	}

	var files []File

	dir := cwd
	for {
		fileForm := firstExisting(dir, localNames)
		dirForm := firstExisting(dir, localDirNames)
		if fileForm != "" && dirForm != "" {
			return nil, fmt.Errorf("both %s and %s exist; keep only one", fileForm, dirForm)
		}
		found := fileForm
		if found == "" {
			found = dirForm
		}
		if found != "" {
			// WorkDir is dir even for the .run form: commands run in
			// the project directory, not inside .run.
			files = append(files, File{Path: found, WorkDir: dir})
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	globalDir, err := GlobalDir()
	if err != nil {
		// Without a home directory there is no global file to merge;
		// only fail if nothing else was found either.
		if len(files) > 0 {
			return files, nil
		}
		return nil, err
	}
	if candidate := firstExisting(globalDir, globalNames); candidate != "" {
		files = append(files, File{Path: candidate, WorkDir: cwd, Global: true})
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no command file found (.run.yaml, .run/run.yaml or %s)", filepath.Join(globalDir, "run.yaml"))
	}
	return files, nil
}

// firstExisting returns the first name under dir that exists as a
// regular file, or "" when none does.
func firstExisting(dir string, names []string) string {
	for _, name := range names {
		candidate := filepath.Join(dir, name)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate
		}
	}
	return ""
}
