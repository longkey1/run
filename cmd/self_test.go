package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// execRoot executes the real root command with the given args against a
// temp command file, capturing stdout. It exercises cobra's routing, so
// "self" resolves to the built-in subcommands while everything else
// falls through to user-defined commands.
func execRoot(t *testing.T, args []string) (string, error) {
	t.Helper()
	return execRootWith(t, testCommands, args)
}

// execRootWith is execRoot with a custom command file content.
func execRootWith(t *testing.T, content string, args []string) (string, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".run.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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

func TestRootNoArgsListsCommands(t *testing.T) {
	out, err := execRoot(t, []string{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out, "deploy <env> [region]") {
		t.Errorf("Execute() output = %q, want command list", out)
	}
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

// TestSelfListJSON checks the machine-readable command tree: one entry
// per command (groups included) with the effective execution context,
// dynamic values unevaluated.
func TestSelfListJSON(t *testing.T) {
	const content = `
shell: bash
env:
  TOP: top
x-note: extension keys are ignored
commands:
  group:
    description: Group desc
    inherit_env: false
    pass_env: [AWS_*]
    env:
      SCOPE: group
    commands:
      sub:
        shell: zsh
        env:
          SCOPE:
            run: echo dyn
        arguments:
          - name: req
            description: required arg
          - name: target
            default:
              run: date +%F
        options:
          - name: force
            type: bool
          - name: from
            default: "2026-01-01"
        run: ./sub.sh
  plain:
    run: echo plain
`
	out, err := execRootWith(t, content, []string{"self", "list", "--json"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", out, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e["name"].(string))
	}
	if want := []string{"group", "group sub", "plain"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	group, sub, plain := entries[0], entries[1], entries[2]

	// The group entry has no run string and carries its own settings.
	if _, ok := group["run"]; ok {
		t.Errorf("group run = %v, want omitted", group["run"])
	}
	if group["description"] != "Group desc" || group["shell"] != "bash" || group["inherit_env"] != false {
		t.Errorf("group = %v, want description/shell/inherit_env from declarations", group)
	}
	if want := map[string]any{"TOP": "top", "SCOPE": "group"}; !reflect.DeepEqual(group["env"], want) {
		t.Errorf("group env = %v, want %v", group["env"], want)
	}

	// The subcommand entry shows the effective context: inner shell and
	// env override, isolation settings inherit, dynamic values stay
	// unevaluated as {"run": ...}.
	if sub["run"] != "./sub.sh" || sub["shell"] != "zsh" || sub["inherit_env"] != false {
		t.Errorf("sub = %v, want run/shell/inherit_env resolved", sub)
	}
	if want := []any{"AWS_*"}; !reflect.DeepEqual(sub["pass_env"], want) {
		t.Errorf("sub pass_env = %v, want %v", sub["pass_env"], want)
	}
	wantEnv := map[string]any{"TOP": "top", "SCOPE": map[string]any{"run": "echo dyn"}}
	if !reflect.DeepEqual(sub["env"], wantEnv) {
		t.Errorf("sub env = %v, want %v", sub["env"], wantEnv)
	}
	wantArgs := []any{
		map[string]any{"name": "req", "description": "required arg", "required": true},
		map[string]any{"name": "target", "required": false, "default": map[string]any{"run": "date +%F"}},
	}
	if !reflect.DeepEqual(sub["arguments"], wantArgs) {
		t.Errorf("sub arguments = %v, want %v", sub["arguments"], wantArgs)
	}
	wantOptions := []any{
		map[string]any{"name": "force", "type": "bool"},
		map[string]any{"name": "from", "type": "string", "default": "2026-01-01"},
	}
	if !reflect.DeepEqual(sub["options"], wantOptions) {
		t.Errorf("sub options = %v, want %v", sub["options"], wantOptions)
	}

	// A command without isolation settings inherits everything; the "sh"
	// default would materialize if no shell were declared.
	if plain["inherit_env"] != true || plain["shell"] != "bash" {
		t.Errorf("plain = %v, want inherit_env true, shell bash", plain)
	}
	if _, ok := plain["pass_env"]; ok {
		t.Errorf("plain pass_env = %v, want omitted", plain["pass_env"])
	}
	if !strings.HasSuffix(plain["source"].(string), ".run.yaml") {
		t.Errorf("plain source = %v, want the command file path", plain["source"])
	}
}

// execLint runs runLint against a temp command file, capturing stdout
// and stderr separately (errors are reported on stderr).
func execLint(t *testing.T, content string) (stdout, stderr string, err error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".run.yaml")
	if werr := os.WriteFile(path, []byte(content), 0o644); werr != nil {
		t.Fatal(werr)
	}
	t.Setenv("RUN_CONFIG", path)

	cmd := &cobra.Command{}
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err = runLint(cmd)
	return out.String(), errOut.String(), err
}

func TestSelfLint(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		out, _, err := execLint(t, "commands:\n  build:\n    run: go build\n")
		if err != nil {
			t.Fatalf("runLint() error = %v", err)
		}
		if want := "ok: 1 command file(s) valid\n"; out != want {
			t.Errorf("runLint() output = %q, want %q", out, want)
		}
	})

	t.Run("broken file reports all errors", func(t *testing.T) {
		_, stderr, err := execLint(t, `
commands:
  a:
    description: no run
  b:
    run: echo
    options:
      - name: force
        type: int
`)
		if err == nil || !strings.Contains(err.Error(), "1 of 1 command file(s) failed") {
			t.Fatalf("runLint() error = %v, want failure summary", err)
		}
		for _, want := range []string{
			`command "a" has no run or subcommands`,
			`option "force" has invalid type "int"`,
		} {
			if !strings.Contains(stderr, want) {
				t.Errorf("runLint() stderr = %q, want it to contain %q", stderr, want)
			}
		}
	})

	t.Run("nothing is executed", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "ran")
		_, _, err := execLint(t, `
env:
  X:
    run: touch `+marker+`
commands:
  a:
    arguments:
      - name: v
        default:
          run: touch `+marker+`
    run: echo
`)
		if err != nil {
			t.Fatalf("runLint() error = %v", err)
		}
		if _, serr := os.Stat(marker); serr == nil {
			t.Error("runLint() evaluated a dynamic value")
		}
	})
}

// Lint keeps going past a broken file, so a broken local and a broken
// global file are both reported in one pass — unlike execution, which
// stops at the first broken file.
func TestSelfLintReportsAllFiles(t *testing.T) {
	setupMerge(t,
		"commands:\n  a:\n    description: broken local\n",
		"commands:\n  b:\n    description: broken global\n",
	)

	cmd := &cobra.Command{}
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := runLint(cmd)
	if err == nil || !strings.Contains(err.Error(), "2 of 2 command file(s) failed") {
		t.Fatalf("runLint() error = %v, want both files to fail", err)
	}
	for _, want := range []string{`command "a" has no run`, `command "b" has no run`} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("runLint() stderr = %q, want it to contain %q", errOut.String(), want)
		}
	}
}

func TestSelfCompletionShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			out, err := execRoot(t, []string{"self", "completion", shell})
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !strings.Contains(out, "run") {
				t.Errorf("Execute() output = %q, want a completion script for run", out)
			}
		})
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
