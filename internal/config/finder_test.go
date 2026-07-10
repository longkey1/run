package config

import (
	"path/filepath"
	"strings"
	"testing"
)

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

		gotPath, gotWorkDir, err := Find(cwd)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if gotPath != cmdFile {
			t.Errorf("Find() path = %q, want %q", gotPath, cmdFile)
		}
		if want := filepath.Join(root, "project"); gotWorkDir != want {
			t.Errorf("Find() workDir = %q, want %q", gotWorkDir, want)
		}
	})

	t.Run(".run.yml is accepted", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", filepath.Join(root, "home"))

		cmdFile := filepath.Join(root, "project", ".run.yml")
		writeFile(t, cmdFile, "commands: {}\n")

		gotPath, _, err := Find(filepath.Join(root, "project"))
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if gotPath != cmdFile {
			t.Errorf("Find() path = %q, want %q", gotPath, cmdFile)
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

		gotPath, gotWorkDir, err := Find(cwd)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if gotPath != globalFile {
			t.Errorf("Find() path = %q, want %q", gotPath, globalFile)
		}
		if gotWorkDir != cwd {
			t.Errorf("Find() workDir = %q, want %q", gotWorkDir, cwd)
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

		gotPath, gotWorkDir, err := Find(cwd)
		if err != nil {
			t.Fatalf("Find() error = %v", err)
		}
		if gotPath != envFile {
			t.Errorf("Find() path = %q, want %q", gotPath, envFile)
		}
		if gotWorkDir != cwd {
			t.Errorf("Find() workDir = %q, want %q", gotWorkDir, cwd)
		}
	})

	t.Run("RUN_CONFIG pointing to missing file is an error", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("HOME", filepath.Join(root, "home"))
		t.Setenv("RUN_CONFIG", filepath.Join(root, "nosuch.yaml"))

		if _, _, err := Find(root); err == nil {
			t.Error("Find() error = nil, want error")
		}
	})

	t.Run("no command file found", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RUN_CONFIG", "")
		t.Setenv("HOME", filepath.Join(root, "home"))

		cwd := filepath.Join(root, "empty")
		writeFile(t, filepath.Join(cwd, ".keep"), "")

		_, _, err := Find(cwd)
		if err == nil {
			t.Fatal("Find() error = nil, want error")
		}
		if !strings.Contains(err.Error(), ".run.yaml") {
			t.Errorf("Find() error = %q, want mention of .run.yaml", err)
		}
	})
}
