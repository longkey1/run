package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// execRoot executes the real root command with the given args against a
// temp command file, capturing stdout. It exercises cobra's routing, so
// "self" resolves to the built-in subcommands while everything else
// falls through to user-defined commands.
func execRoot(t *testing.T, args []string) (string, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".run.yaml")
	if err := os.WriteFile(path, []byte(testCommands), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUN_CONFIG", path)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs(args)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	}()
	err := rootCmd.Execute()
	return out.String(), err
}

func TestSelfVersion(t *testing.T) {
	out, err := execRoot(t, []string{"self", "version"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out, "Version:") {
		t.Errorf("Execute() output = %q, want version info", out)
	}
}

func TestSelfList(t *testing.T) {
	out, err := execRoot(t, []string{"self", "list"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out, "deploy <env> [region]") {
		t.Errorf("Execute() output = %q, want command list", out)
	}
	if !strings.Contains(out, "optcmd <target> [--force] [--from <from>] [--label <label>]") {
		t.Errorf("Execute() output = %q, want option signature", out)
	}
}

func TestSelfCompletionUnsupportedShell(t *testing.T) {
	_, err := execRoot(t, []string{"self", "completion", "tcsh"})
	if err == nil || !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("Execute() error = %v, want unsupported shell", err)
	}
}

func TestSelfDoesNotShadowUserArguments(t *testing.T) {
	// "self" is only reserved as the first argument; deeper in the
	// path it is an ordinary argument or subcommand name.
	out, err := execRoot(t, []string{"echo", "self"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := "self\n"; out != want {
		t.Errorf("Execute() output = %q, want %q", out, want)
	}
}

// execSelfPath executes the real root command without touching
// RUN_CONFIG or the working directory, returning stdout with the
// trailing newline stripped.
func execSelfPath(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs(args)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	}()
	err := rootCmd.Execute()
	return strings.TrimSuffix(out.String(), "\n"), err
}

// writeLocal writes a local command file at path (creating parent
// directories) with a minimal valid definition.
func writeLocal(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("commands:\n  x:\n    run: echo x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// canon resolves symlinks so paths compare stably: t.TempDir may live
// behind a symlink (e.g. /var -> /private/var on darwin), and whether
// os.Getwd resolves it depends on $PWD.
func canon(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestSelfPathFileForm(t *testing.T) {
	t.Setenv("RUN_CONFIG", "")
	root := t.TempDir()
	writeLocal(t, filepath.Join(root, ".run.yaml"))
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	want := canon(t, root)

	// The file form has no separate config directory: root, local, and
	// the default target all print the directory containing .run.yaml.
	for _, args := range [][]string{
		{"self", "path"},
		{"self", "path", "root"},
		{"self", "path", "local"},
	} {
		out, err := execSelfPath(t, args...)
		if err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
		if canon(t, out) != want {
			t.Errorf("Execute(%v) = %q, want %q", args, out, want)
		}
	}
}

func TestSelfPathDirForm(t *testing.T) {
	t.Setenv("RUN_CONFIG", "")
	root := t.TempDir()
	writeLocal(t, filepath.Join(root, ".run", "run.yaml"))
	t.Chdir(root)
	want := canon(t, root)

	out, err := execSelfPath(t, "self", "path", "root")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if canon(t, out) != want {
		t.Errorf("Execute() root = %q, want %q", out, want)
	}

	out, err = execSelfPath(t, "self", "path", "local")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if wantLocal := filepath.Join(want, ".run"); canon(t, out) != wantLocal {
		t.Errorf("Execute() local = %q, want %q", out, wantLocal)
	}
}

func TestSelfPathGlobal(t *testing.T) {
	t.Setenv("RUN_CONFIG", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	// The global directory is a fixed location: printed even though
	// nothing exists under it yet.
	out, err := execSelfPath(t, "self", "path", "global")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := filepath.Join(home, ".config", "run"); out != want {
		t.Errorf("Execute() global = %q, want %q", out, want)
	}
}

func TestSelfPathRunConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "custom.yaml")
	writeLocal(t, cfg)
	t.Setenv("RUN_CONFIG", cfg)
	cwd := t.TempDir()
	t.Chdir(cwd)

	// A $RUN_CONFIG file counts as local: root is the current
	// directory (where its commands run), local is the file's
	// directory, and the unused global directory is an error.
	out, err := execSelfPath(t, "self", "path", "root")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := canon(t, cwd); canon(t, out) != want {
		t.Errorf("Execute() root = %q, want %q", out, want)
	}

	out, err = execSelfPath(t, "self", "path", "local")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := canon(t, dir); canon(t, out) != want {
		t.Errorf("Execute() local = %q, want %q", out, want)
	}

	if _, err := execSelfPath(t, "self", "path", "global"); err == nil || !strings.Contains(err.Error(), "RUN_CONFIG") {
		t.Fatalf("Execute() global error = %v, want RUN_CONFIG error", err)
	}
}

func TestSelfPathNoLocalFile(t *testing.T) {
	t.Setenv("RUN_CONFIG", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A global file alone does not satisfy root/local.
	writeLocal(t, filepath.Join(home, ".config", "run", "run.yaml"))
	t.Chdir(t.TempDir())

	if _, err := execSelfPath(t, "self", "path", "local"); err == nil || !strings.Contains(err.Error(), "no local command file") {
		t.Fatalf("Execute() error = %v, want no local command file", err)
	}
}

func TestSelfPathInvalidTarget(t *testing.T) {
	if _, err := execSelfPath(t, "self", "path", "bogus"); err == nil || !strings.Contains(err.Error(), "invalid argument") {
		t.Fatalf("Execute() error = %v, want invalid argument", err)
	}
}
