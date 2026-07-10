package cmd

import (
	"bytes"
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
`

// execCommand runs runCommand against a temp command file and captures
// stdout.
func execCommand(t *testing.T, args []string) (string, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".run.yaml")
	if err := os.WriteFile(path, []byte(testCommands), 0o644); err != nil {
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
