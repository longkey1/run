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
