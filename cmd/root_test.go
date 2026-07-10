package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const testCommands = `
env:
  GREETING: hello
  SCOPE: top
commands:
  echo:
    run: printf '%s\n' "$@"
  showenv:
    run: echo "$GREETING $SCOPE"
  envcmd:
    env:
      SCOPE: command
      NAME: world
    run: echo "$GREETING $SCOPE $NAME"
  envgroup:
    env:
      SCOPE: group
      GROUP: g
    commands:
      inner:
        env:
          SCOPE: inner
        run: echo "$GREETING $SCOPE $GROUP"
  envarg:
    env:
      target: from-env
    args:
      - name: target
    run: echo "$target"
  deploy:
    args:
      - name: env
      - name: region
        default: us-east-1
    run: echo "$env/$region $1/$2"
  wrap:
    args:
      - name: first
    run: printf '%s\n' "$@"
  db:
    run: echo "db status $1"
    commands:
      migrate:
        run: echo migrate
  group:
    commands:
      sub:
        run: echo sub
  flagcmd:
    env:
      from: from-env
    args:
      - name: target
    flags:
      - name: force
        type: bool
      - name: from
        default: "2026-01-01"
      - name: label
    run: printf '%s\n' "force=$force from=$from label=$label" "$@"
  dynenv:
    env:
      STAMP:
        run: echo dyn-value
      COMBO:
        run: echo "$GREETING-x"
    run: echo "$STAMP $COMBO"
  dyndefault:
    env:
      TODAY:
        run: printf 2026-07-10
    args:
      - name: date
        default:
          run: printf '%s' "$TODAY"
    flags:
      - name: from
        default:
          run: printf f-default
    run: printf '%s\n' "date=$date from=$from" "$@"
  dynskip:
    args:
      - name: v
        default:
          run: 'echo ran > "$MARKER"; printf d'
    run: printf '%s\n' "$1"
  dynfail:
    env:
      BAD:
        run: exit 3
    run: echo unreachable
  dynfaildefault:
    args:
      - name: v
        default:
          run: exit 2
    run: echo unreachable
  dynoverride:
    env:
      X:
        run: exit 1
    commands:
      inner:
        env:
          X: literal
        run: echo "$X"
  described:
    description: Deploy the app
    args:
      - name: env
        description: target environment
    flags:
      - name: force
        type: bool
        description: skip confirmation
      - name: from
        default: "2026-01-01"
    run: echo x
  helpflag:
    flags:
      - name: help
        type: bool
    run: printf '%s\n' "help=$help" "$@"
`

// execCommand runs runCommand against a temp command file holding
// testCommands and captures stdout.
func execCommand(t *testing.T, args []string) (string, error) {
	t.Helper()
	return execCommandWith(t, testCommands, args)
}

// execCommandWith runs runCommand against a temp command file with the
// given content and captures stdout.
func execCommandWith(t *testing.T, content string, args []string) (string, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".run.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUN_CONFIG", path)

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	err := runCommand(cmd, args)
	return out.String(), err
}

func TestRunCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOut string
		wantErr string // substring; empty means success
	}{
		{
			name:    "args pass through via $@",
			args:    []string{"echo", "a", "b c"},
			wantOut: "a\nb c\n",
		},
		{
			name:    "subcommand wins over argument",
			args:    []string{"db", "migrate"},
			wantOut: "migrate\n",
		},
		{
			name:    "unmatched name becomes argument",
			args:    []string{"db", "foo"},
			wantOut: "db status foo\n",
		},
		{
			name:    "explicit -- forces argument",
			args:    []string{"db", "--", "migrate"},
			wantOut: "db status migrate\n",
		},
		{
			name:    "default fills env var and positional",
			args:    []string{"deploy", "prod"},
			wantOut: "prod/us-east-1 prod/us-east-1\n",
		},
		{
			name:    "explicit value overrides default",
			args:    []string{"deploy", "prod", "jp"},
			wantOut: "prod/jp prod/jp\n",
		},
		{
			name:    "missing required argument",
			args:    []string{"deploy"},
			wantErr: `missing required argument "env"`,
		},
		{
			name:    "extra args beyond declaration pass through",
			args:    []string{"wrap", "a", "b"},
			wantOut: "a\nb\n",
		},
		{
			name:    "group rejects unknown subcommand",
			args:    []string{"group", "x"},
			wantErr: `command "group" has no subcommand "x"`,
		},
		{
			name:    "group rejects explicit arguments",
			args:    []string{"group", "--", "x"},
			wantErr: `command "group" has no run`,
		},
		{
			name:    "unknown command",
			args:    []string{"nope"},
			wantErr: `command "nope" not found`,
		},
		{
			name:    "-- before resolved path still errors",
			args:    []string{"db", "nope", "--", "x"},
			wantErr: `command "db" has no subcommand "nope"`,
		},
		{
			name:    "top-level env applies to command",
			args:    []string{"showenv"},
			wantOut: "hello top\n",
		},
		{
			name:    "command env overrides top-level",
			args:    []string{"envcmd"},
			wantOut: "hello command world\n",
		},
		{
			name:    "nested command inherits ancestor env and overrides",
			args:    []string{"envgroup", "inner"},
			wantOut: "hello inner g\n",
		},
		{
			name:    "declared arg overrides command env",
			args:    []string{"envarg", "cli"},
			wantOut: "cli\n",
		},
		{
			name:    "bool flag set and normalized after positionals",
			args:    []string{"flagcmd", "t", "--force"},
			wantOut: "force=true from=2026-01-01 label=\nt\n--force\n--from\n2026-01-01\n",
		},
		{
			name:    "bool flag unset and flag default overrides command env",
			args:    []string{"flagcmd", "t"},
			wantOut: "force=false from=2026-01-01 label=\nt\n--from\n2026-01-01\n",
		},
		{
			name:    "value flag space form",
			args:    []string{"flagcmd", "t", "--from", "2026-04-01"},
			wantOut: "force=false from=2026-04-01 label=\nt\n--from\n2026-04-01\n",
		},
		{
			name:    "value flag equals form",
			args:    []string{"flagcmd", "t", "--from=2026-04-01"},
			wantOut: "force=false from=2026-04-01 label=\nt\n--from\n2026-04-01\n",
		},
		{
			name:    "flag before positional keeps $1 stable",
			args:    []string{"flagcmd", "--force", "t"},
			wantOut: "force=true from=2026-01-01 label=\nt\n--force\n--from\n2026-01-01\n",
		},
		{
			name:    "unknown flag",
			args:    []string{"flagcmd", "t", "--bogus"},
			wantErr: `unknown flag --bogus`,
		},
		{
			name:    "bool flag rejects a value",
			args:    []string{"flagcmd", "t", "--force=yes"},
			wantErr: `flag --force does not take a value`,
		},
		{
			name:    "value flag missing value",
			args:    []string{"flagcmd", "t", "--from"},
			wantErr: `flag --from requires a value`,
		},
		{
			name:    "repeated flag last wins",
			args:    []string{"flagcmd", "t", "--from", "a", "--from", "b"},
			wantOut: "force=false from=b label=\nt\n--from\nb\n",
		},
		{
			name:    "space form value taken literally",
			args:    []string{"flagcmd", "t", "--label", "--force"},
			wantOut: "force=false from=2026-01-01 label=--force\nt\n--from\n2026-01-01\n--label\n--force\n",
		},
		{
			name:    "single dash token is positional",
			args:    []string{"flagcmd", "-x"},
			wantOut: "force=false from=2026-01-01 label=\n-x\n--from\n2026-01-01\n",
		},
		{
			name:    "tokens after -- are literal even for flag command",
			args:    []string{"flagcmd", "--force", "--", "--bogus"},
			wantOut: "force=true from=2026-01-01 label=\n--bogus\n--force\n--from\n2026-01-01\n",
		},
		{
			name:    "bare token before -- still errors on flag command",
			args:    []string{"flagcmd", "x", "--force", "--", "y"},
			wantErr: `command "flagcmd" has no subcommand "x"`,
		},
		{
			name:    "missing required arg with only flags",
			args:    []string{"flagcmd", "--force"},
			wantErr: `missing required argument "target"`,
		},
		{
			name:    "command without flags passes --tokens through",
			args:    []string{"echo", "--whatever", "-x"},
			wantOut: "--whatever\n-x\n",
		},
		{
			name:    "dynamic env resolved with trailing newline trimmed",
			args:    []string{"dynenv"},
			wantOut: "dyn-value hello-x\n",
		},
		{
			name:    "dynamic defaults fill env vars and positionals",
			args:    []string{"dyndefault"},
			wantOut: "date=2026-07-10 from=f-default\n2026-07-10\n--from\nf-default\n",
		},
		{
			name:    "explicit values override dynamic defaults",
			args:    []string{"dyndefault", "2020-01-01", "--from", "cli"},
			wantOut: "date=2020-01-01 from=cli\n2020-01-01\n--from\ncli\n",
		},
		{
			name:    "dynamic env failure",
			args:    []string{"dynfail"},
			wantErr: `command "dynfail": env "BAD"`,
		},
		{
			name:    "dynamic default failure",
			args:    []string{"dynfaildefault"},
			wantErr: `command "dynfaildefault": default for argument "v"`,
		},
		{
			name:    "overridden dynamic env never runs",
			args:    []string{"dynoverride", "inner"},
			wantOut: "literal\n",
		},
		{
			name:    "--help after -- is a literal positional",
			args:    []string{"echo", "--", "--help"},
			wantOut: "--help\n",
		},
		{
			name:    "-h is a positional, not help",
			args:    []string{"echo", "-h"},
			wantOut: "-h\n",
		},
		{
			name:    "declared help flag disables interception",
			args:    []string{"helpflag", "--help"},
			wantOut: "help=true\n--help\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := execCommand(t, tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("runCommand() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("runCommand() error = %v", err)
			}
			if out != tt.wantOut {
				t.Errorf("runCommand() output = %q, want %q", out, tt.wantOut)
			}
		})
	}
}

func TestRunCommandHelp(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{
			name: "full help with descriptions and default",
			args: []string{"described", "--help"},
			wantOut: `Deploy the app

Usage:
  run described <env> [--force] [--from <from>]

Arguments:
  <env>  target environment

Options:
  --force        skip confirmation
  --from <from>  (default: 2026-01-01)
  --help         show this help
`,
		},
		{
			name: "rows without details stay unpadded",
			args: []string{"flagcmd", "--help"},
			wantOut: `Usage:
  run flagcmd <target> [--force] [--from <from>] [--label <label>]

Arguments:
  <target>

Options:
  --force
  --from <from>  (default: 2026-01-01)
  --label <label>
  --help         show this help
`,
		},
		{
			name: "args only command gets the implicit --help option",
			args: []string{"deploy", "--help"},
			wantOut: `Usage:
  run deploy <env> [region]

Arguments:
  <env>
  [region]  (default: us-east-1)

Options:
  --help  show this help
`,
		},
		{
			name: "group lists its subcommands",
			args: []string{"group", "--help"},
			wantOut: `Usage:
  run group <command>

Commands:
  group sub
`,
		},
		{
			name: "runnable group gets both usage lines",
			args: []string{"db", "--help"},
			wantOut: `Usage:
  run db
  run db <command>

Options:
  --help  show this help

Commands:
  db migrate
`,
		},
		{
			name: "dynamic defaults are labeled, not executed",
			args: []string{"dyndefault", "--help"},
			wantOut: `Usage:
  run dyndefault [date] [--from <from>]

Arguments:
  [date]  (default: dynamic)

Options:
  --from <from>  (default: dynamic)
  --help         show this help
`,
		},
		{
			name: "--help anywhere before -- intercepts",
			args: []string{"echo", "x", "--help"},
			wantOut: `Usage:
  run echo

Options:
  --help  show this help
`,
		},
		{
			name: "--help wins even as a would-be flag value",
			args: []string{"flagcmd", "t", "--label", "--help"},
			wantOut: `Usage:
  run flagcmd <target> [--force] [--from <from>] [--label <label>]

Arguments:
  <target>

Options:
  --force
  --from <from>  (default: 2026-01-01)
  --label <label>
  --help         show this help
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := execCommand(t, tt.args)
			if err != nil {
				t.Fatalf("runCommand() error = %v", err)
			}
			if out != tt.wantOut {
				t.Errorf("runCommand() output = %q, want %q", out, tt.wantOut)
			}
		})
	}
}

// TestRunCommandHelpNeverRunsDynamic verifies that showing help does
// not evaluate dynamic env values or defaults.
func TestRunCommandHelpNeverRunsDynamic(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("MARKER", marker)

	if _, err := execCommand(t, []string{"dynskip", "--help"}); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("dynamic default ran while rendering help (marker stat err = %v)", err)
	}
}

// TestRunCommandDynamicDefaultLazy verifies that a dynamic default's
// command only runs when the default is actually used.
func TestRunCommandDynamicDefaultLazy(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("MARKER", marker)

	out, err := execCommand(t, []string{"dynskip", "explicit"})
	if err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if want := "explicit\n"; out != want {
		t.Errorf("runCommand() output = %q, want %q", out, want)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("default command ran despite explicit value (marker stat err = %v)", err)
	}

	out, err = execCommand(t, []string{"dynskip"})
	if err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if want := "d\n"; out != want {
		t.Errorf("runCommand() output = %q, want %q", out, want)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("default command did not run: %v", err)
	}
}

// TestRunCommandShell verifies that run strings and dynamic values
// execute with the configured shell: the top-level shell by default, a
// command's own shell when declared, inherited through groups.
func TestRunCommandShell(t *testing.T) {
	// A stub shell that marks itself via FAKESHELL and delegates to sh,
	// so run strings behave normally but reveal which shell ran them.
	stub := func(mark string) string {
		path := filepath.Join(t.TempDir(), mark)
		script := "#!/bin/sh\nFAKESHELL=" + mark + " exec sh \"$@\"\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	content := fmt.Sprintf(`
shell: %s
commands:
  top:
    run: echo "$FAKESHELL"
  over:
    shell: %s
    run: echo "$FAKESHELL"
  group:
    shell: %s
    commands:
      inner:
        run: echo "$FAKESHELL"
  dyn:
    env:
      V:
        run: printf "dyn-$FAKESHELL"
    args:
      - name: a
        default:
          run: printf "def-$FAKESHELL"
    run: echo "$V $a"
`, stub("one"), stub("two"), stub("three"))

	tests := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{
			name:    "top-level shell runs the command",
			args:    []string{"top"},
			wantOut: "one\n",
		},
		{
			name:    "command shell overrides top-level",
			args:    []string{"over"},
			wantOut: "two\n",
		},
		{
			name:    "nested command inherits group shell",
			args:    []string{"group", "inner"},
			wantOut: "three\n",
		},
		{
			name:    "dynamic env and defaults use the resolved shell",
			args:    []string{"dyn"},
			wantOut: "dyn-one def-one\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := execCommandWith(t, content, tt.args)
			if err != nil {
				t.Fatalf("runCommand() error = %v", err)
			}
			if out != tt.wantOut {
				t.Errorf("runCommand() output = %q, want %q", out, tt.wantOut)
			}
		})
	}
}

func TestRunCommandEnvOverridesOS(t *testing.T) {
	t.Setenv("SCOPE", "os")

	out, err := execCommand(t, []string{"showenv"})
	if err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if want := "hello top\n"; out != want {
		t.Errorf("runCommand() output = %q, want %q", out, want)
	}
}
