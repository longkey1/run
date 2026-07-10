package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const mergeLocalCommands = `
env:
  ORIGIN: local
commands:
  build:
    description: local build
    run: echo "local build"
  where:
    run: pwd -P
  origin:
    run: echo "local $ORIGIN"
`

const mergeGlobalCommands = `
env:
  ORIGIN: global
  GLOBAL_ONLY: g
commands:
  build:
    description: global build
    run: echo "global build"
  gwhere:
    run: pwd -P
  gorigin:
    run: echo "global $ORIGIN $GLOBAL_ONLY"
  gleak:
    run: echo "leak=$ORIGIN"
`

// setupMerge creates a global command file under a temp HOME and a
// local .run.yaml in a project directory, then chdirs into a
// subdirectory of the project. RUN_CONFIG is cleared so both files
// resolve and merge. It returns the project directory (the local
// commands' workDir) and the cwd (the global commands' workDir).
func setupMerge(t *testing.T, localContent, globalContent string) (projectDir, cwd string) {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("RUN_CONFIG", "")
	t.Setenv("HOME", home)

	if globalContent != "" {
		globalFile := filepath.Join(home, ".config", "run", "run.yaml")
		if err := os.MkdirAll(filepath.Dir(globalFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(globalFile, []byte(globalContent), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	projectDir = filepath.Join(root, "project")
	cwd = filepath.Join(projectDir, "sub")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if localContent != "" {
		if err := os.WriteFile(filepath.Join(projectDir, ".run.yaml"), []byte(localContent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(cwd)

	// Resolve symlinks (macOS /var -> /private/var) so pwd output from
	// executed commands compares equal.
	if projectDir, err := filepath.EvalSymlinks(projectDir); err == nil {
		if cwd2, err := filepath.EvalSymlinks(cwd); err == nil {
			return projectDir, cwd2
		}
	}
	return projectDir, cwd
}

// execMerged runs runCommand in the merge setup and captures stdout.
func execMerged(t *testing.T, args []string) (string, error) {
	t.Helper()
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	err := runCommand(cmd, args)
	return out.String(), err
}

func TestMergeGlobalCommands(t *testing.T) {
	t.Run("global command is callable where a local file exists", func(t *testing.T) {
		setupMerge(t, mergeLocalCommands, mergeGlobalCommands)
		out, err := execMerged(t, []string{"gorigin"})
		if err != nil {
			t.Fatalf("runCommand() error = %v", err)
		}
		if got, want := strings.TrimSpace(out), "global global g"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("local command shadows a same-named global command", func(t *testing.T) {
		setupMerge(t, mergeLocalCommands, mergeGlobalCommands)
		out, err := execMerged(t, []string{"build"})
		if err != nil {
			t.Fatalf("runCommand() error = %v", err)
		}
		if got, want := strings.TrimSpace(out), "local build"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("local commands run in the local file's directory", func(t *testing.T) {
		projectDir, _ := setupMerge(t, mergeLocalCommands, mergeGlobalCommands)
		out, err := execMerged(t, []string{"where"})
		if err != nil {
			t.Fatalf("runCommand() error = %v", err)
		}
		if got := strings.TrimSpace(out); got != projectDir {
			t.Errorf("local workDir = %q, want %q", got, projectDir)
		}
	})

	t.Run("global commands run in the current directory", func(t *testing.T) {
		_, cwd := setupMerge(t, mergeLocalCommands, mergeGlobalCommands)
		out, err := execMerged(t, []string{"gwhere"})
		if err != nil {
			t.Fatalf("runCommand() error = %v", err)
		}
		if got := strings.TrimSpace(out); got != cwd {
			t.Errorf("global workDir = %q, want %q", got, cwd)
		}
	})

	t.Run("top-level env stays with its own file", func(t *testing.T) {
		setupMerge(t, mergeLocalCommands, mergeGlobalCommands)
		out, err := execMerged(t, []string{"origin"})
		if err != nil {
			t.Fatalf("runCommand() error = %v", err)
		}
		// The local command sees the local ORIGIN, not the global one,
		// and the global GLOBAL_ONLY does not leak in.
		if got, want := strings.TrimSpace(out), "local local"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("list shows merged commands without shadowed globals", func(t *testing.T) {
		setupMerge(t, mergeLocalCommands, mergeGlobalCommands)
		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		if err := runList(cmd); err != nil {
			t.Fatalf("runList() error = %v", err)
		}
		got := out.String()
		for _, want := range []string{"build", "where", "origin", "gwhere", "gorigin"} {
			if !strings.Contains(got, want) {
				t.Errorf("list output missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "global build") {
			t.Errorf("list output shows shadowed global command:\n%s", got)
		}
		if !strings.Contains(got, "local build") {
			t.Errorf("list output missing local command description:\n%s", got)
		}
	})

	t.Run("completion offers merged top-level names", func(t *testing.T) {
		setupMerge(t, mergeLocalCommands, mergeGlobalCommands)
		names, directive := completeCommands(rootCmd, nil, "")
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("completeCommands() directive = %v, want NoFileComp", directive)
		}
		stripped := make([]string, len(names))
		for i, n := range names {
			stripped[i], _, _ = strings.Cut(n, "\t")
		}
		slices.Sort(stripped)
		want := []string{"build", "gleak", "gorigin", "gwhere", "origin", "where"}
		if !slices.Equal(stripped, want) {
			t.Errorf("completion candidates = %v, want %v", stripped, want)
		}
		// The shadowed global "build" must not produce a duplicate with
		// the global description.
		if slices.Contains(names, "build\tglobal build") {
			t.Errorf("completion offers shadowed global command: %v", names)
		}
	})

	t.Run("RUN_CONFIG disables merging", func(t *testing.T) {
		_, cwd := setupMerge(t, mergeLocalCommands, mergeGlobalCommands)
		t.Setenv("RUN_CONFIG", filepath.Join(cwd, "..", ".run.yaml"))
		if _, err := execMerged(t, []string{"gorigin"}); err == nil {
			t.Error("runCommand() error = nil, want not-found error for global command")
		}
	})

	t.Run("broken global file is a hard error", func(t *testing.T) {
		setupMerge(t, mergeLocalCommands, "commands: {broken\n")
		if _, err := execMerged(t, []string{"build"}); err == nil {
			t.Error("runCommand() error = nil, want parse error for broken global file")
		}
	})
}
