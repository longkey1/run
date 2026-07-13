package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// singleFile asserts that files holds exactly one entry and
// returns it.
func singleFile(t *testing.T, files []File) File {
	t.Helper()
	if len(files) != 1 {
		t.Fatalf("Find() returned %d files, want 1: %v", len(files), files)
	}
	return files[0]
}

// Find depends on process-wide state (environment variables), so these
// subtests use t.Setenv and must not run in parallel.
func TestFind(t *testing.T) {
	t.Run("local file found in ancestor directory", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", filepath.Join(root, "home"))

		cmdFile := filepath.Join(root, "project", ".run.yaml")
		writeFile(t, cmdFile, "commands: {}\n")

		cwd := filepath.Join(root, "project", "sub", "deep")
		writeFile(t, filepath.Join(cwd, ".keep"), "")

		sources, err := Find(cwd)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		got := singleFile(t, sources)
		if got.Path != cmdFile {
			t.Errorf("Find() path = %q, want %q", got.Path, cmdFile)
		}
		if want := filepath.Join(root, "project"); got.WorkDir != want {
			t.Errorf("Find() workDir = %q, want %q", got.WorkDir, want)
		}
	})

	t.Run(".run.yml is accepted", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", filepath.Join(root, "home"))

		cmdFile := filepath.Join(root, "project", ".run.yml")
		writeFile(t, cmdFile, "commands: {}\n")

		sources, err := Find(filepath.Join(root, "project"))
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if got := singleFile(t, sources); got.Path != cmdFile {
			t.Errorf("Find() path = %q, want %q", got.Path, cmdFile)
		}
	})

	t.Run(".run/run.yaml is found with the parent as workDir", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", filepath.Join(root, "home"))

		cmdFile := filepath.Join(root, "project", ".run", "run.yaml")
		writeFile(t, cmdFile, "commands: {}\n")

		cwd := filepath.Join(root, "project", "sub")
		writeFile(t, filepath.Join(cwd, ".keep"), "")

		sources, err := Find(cwd)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		got := singleFile(t, sources)
		if got.Path != cmdFile {
			t.Errorf("Find() path = %q, want %q", got.Path, cmdFile)
		}
		if want := filepath.Join(root, "project"); got.WorkDir != want {
			t.Errorf("Find() workDir = %q, want %q", got.WorkDir, want)
		}
	})

	t.Run(".run/run.yml is accepted", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", filepath.Join(root, "home"))

		cmdFile := filepath.Join(root, "project", ".run", "run.yml")
		writeFile(t, cmdFile, "commands: {}\n")

		sources, err := Find(filepath.Join(root, "project"))
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if got := singleFile(t, sources); got.Path != cmdFile {
			t.Errorf("Find() path = %q, want %q", got.Path, cmdFile)
		}
	})

	t.Run("file and directory forms in the same directory are an error", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", filepath.Join(root, "home"))

		project := filepath.Join(root, "project")
		writeFile(t, filepath.Join(project, ".run.yaml"), "commands: {}\n")
		writeFile(t, filepath.Join(project, ".run", "run.yaml"), "commands: {}\n")

		_, err := Find(project)
		if err == nil {
			t.Fatal("Find() error = nil, want error")
		}
		if !strings.Contains(err.Error(), ".run.yaml") || !strings.Contains(err.Error(), filepath.Join(".run", "run.yaml")) {
			t.Errorf("Find() error = %q, want mention of both forms", err)
		}
	})

	t.Run(".run form in a closer directory stops the ancestor search", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", filepath.Join(root, "home"))

		writeFile(t, filepath.Join(root, ".run.yaml"), "commands: {}\n")
		cmdFile := filepath.Join(root, "project", ".run", "run.yaml")
		writeFile(t, cmdFile, "commands: {}\n")

		cwd := filepath.Join(root, "project", "sub")
		writeFile(t, filepath.Join(cwd, ".keep"), "")

		sources, err := Find(cwd)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if got := singleFile(t, sources); got.Path != cmdFile {
			t.Errorf("Find() path = %q, want %q", got.Path, cmdFile)
		}
	})

	t.Run("global fallback keeps cwd as workDir", func(t *testing.T) {
		root := t.TempDir()
		home := filepath.Join(root, "home")
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", home)

		globalFile := filepath.Join(home, ".config", "run", "run.yaml")
		writeFile(t, globalFile, "commands: {}\n")

		cwd := filepath.Join(root, "elsewhere")
		writeFile(t, filepath.Join(cwd, ".keep"), "")

		sources, err := Find(cwd)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		got := singleFile(t, sources)
		if got.Path != globalFile {
			t.Errorf("Find() path = %q, want %q", got.Path, globalFile)
		}
		if got.WorkDir != cwd {
			t.Errorf("Find() workDir = %q, want %q", got.WorkDir, cwd)
		}
	})

	t.Run("local and global files are both returned, local first", func(t *testing.T) {
		root := t.TempDir()
		home := filepath.Join(root, "home")
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", home)

		globalFile := filepath.Join(home, ".config", "run", "run.yaml")
		writeFile(t, globalFile, "commands: {}\n")

		localFile := filepath.Join(root, "project", ".run.yaml")
		writeFile(t, localFile, "commands: {}\n")

		cwd := filepath.Join(root, "project", "sub")
		writeFile(t, filepath.Join(cwd, ".keep"), "")

		sources, err := Find(cwd)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if len(sources) != 2 {
			t.Fatalf("Find() returned %d sources, want 2: %v", len(sources), sources)
		}
		if sources[0].Path != localFile {
			t.Errorf("Find() sources[0].Path = %q, want %q", sources[0].Path, localFile)
		}
		if want := filepath.Join(root, "project"); sources[0].WorkDir != want {
			t.Errorf("Find() sources[0].WorkDir = %q, want %q", sources[0].WorkDir, want)
		}
		if sources[1].Path != globalFile {
			t.Errorf("Find() sources[1].Path = %q, want %q", sources[1].Path, globalFile)
		}
		if sources[1].WorkDir != cwd {
			t.Errorf("Find() sources[1].WorkDir = %q, want %q", sources[1].WorkDir, cwd)
		}
	})

	t.Run("RUN_CONFIG takes precedence over local file", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("HOME", filepath.Join(root, "home"))

		envFile := filepath.Join(root, "custom.yaml")
		writeFile(t, envFile, "commands: {}\n")
		t.Setenv("RUN_CONFIG", envFile)

		cwd := filepath.Join(root, "cwd")
		writeFile(t, filepath.Join(cwd, ".run.yaml"), "commands: {}\n")

		sources, err := Find(cwd)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		got := singleFile(t, sources)
		if got.Path != envFile {
			t.Errorf("Find() path = %q, want %q", got.Path, envFile)
		}
		if got.WorkDir != cwd {
			t.Errorf("Find() workDir = %q, want %q", got.WorkDir, cwd)
		}
	})

	t.Run("RUN_CONFIG is used alone even when a global file exists", func(t *testing.T) {
		root := t.TempDir()
		home := filepath.Join(root, "home")
		t.Setenv("HOME", home)

		writeFile(t, filepath.Join(home, ".config", "run", "run.yaml"), "commands: {}\n")

		envFile := filepath.Join(root, "custom.yaml")
		writeFile(t, envFile, "commands: {}\n")
		t.Setenv("RUN_CONFIG", envFile)

		sources, err := Find(root)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if got := singleFile(t, sources); got.Path != envFile {
			t.Errorf("Find() path = %q, want %q", got.Path, envFile)
		}
	})

	t.Run("RUN_CONFIG pointing to missing file is an error", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("HOME", filepath.Join(root, "home"))
		t.Setenv("RUN_CONFIG", filepath.Join(root, "nosuch.yaml"))

		if _, err := Find(root); err == nil {
			t.Error("Find() error = nil, want error")
		}
	})

	t.Run("missing home directory is ignored when a local file exists", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		// An empty HOME makes os.UserHomeDir fail: there is no global
		// file to merge, but the local file still resolves.
		t.Setenv("HOME", "")

		cmdFile := filepath.Join(root, "project", ".run.yaml")
		writeFile(t, cmdFile, "commands: {}\n")

		sources, err := Find(filepath.Join(root, "project"))
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if got := singleFile(t, sources); got.Path != cmdFile {
			t.Errorf("Find() path = %q, want %q", got.Path, cmdFile)
		}
	})

	t.Run("missing home directory with no local file is an error", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", "")

		if _, err := Find(root); err == nil {
			t.Error("Find() error = nil, want error")
		}
	})

	t.Run("no command file found", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", filepath.Join(root, "home"))

		cwd := filepath.Join(root, "empty")
		writeFile(t, filepath.Join(cwd, ".keep"), "")

		_, err := Find(cwd)
		if err == nil {
			t.Fatal("Find() error = nil, want error")
		}
		if !strings.Contains(err.Error(), ".run.yaml") {
			t.Errorf("Find() error = %q, want mention of .run.yaml", err)
		}
	})
}
