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

const testTasks = `
env:
  GREETING: hello
  SCOPE: top
tasks:
  echo:
    command: printf '%s\n' "$@"
  showenv:
    command: echo "$GREETING $SCOPE"
  envtask:
    env:
      SCOPE: task
      NAME: world
    command: echo "$GREETING $SCOPE $NAME"
  envgroup:
    env:
      SCOPE: group
      GROUP: g
    tasks:
      inner:
        env:
          SCOPE: inner
        command: echo "$GREETING $SCOPE $GROUP"
  envarg:
    env:
      target: from-env
    args:
      - name: target
    command: echo "$target"
  deploy:
    args:
      - name: env
      - name: region
        default: us-east-1
    command: echo "$env/$region $1/$2"
  wrap:
    args:
      - name: first
    command: printf '%s\n' "$@"
  db:
    command: echo "db status $1"
    tasks:
      migrate:
        command: echo migrate
  group:
    tasks:
      sub:
        command: echo sub
`

// execTask runs runTask against a temp task file and captures stdout.
func execTask(t *testing.T, args []string) (string, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".run.yaml")
	if err := os.WriteFile(path, []byte(testTasks), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUN_CONFIG", path)

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	err := runTask(cmd, args)
	return out.String(), err
}

func TestRunTask(t *testing.T) {
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
			name:    "subtask wins over argument",
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
			name:    "group rejects unknown subtask",
			args:    []string{"group", "x"},
			wantErr: `task "group" has no subtask "x"`,
		},
		{
			name:    "group rejects explicit arguments",
			args:    []string{"group", "--", "x"},
			wantErr: `task "group" has no command`,
		},
		{
			name:    "unknown task",
			args:    []string{"nope"},
			wantErr: `task "nope" not found`,
		},
		{
			name:    "-- before resolved path still errors",
			args:    []string{"db", "nope", "--", "x"},
			wantErr: `task "db" has no subtask "nope"`,
		},
		{
			name:    "top-level env applies to task",
			args:    []string{"showenv"},
			wantOut: "hello top\n",
		},
		{
			name:    "task env overrides top-level",
			args:    []string{"envtask"},
			wantOut: "hello task world\n",
		},
		{
			name:    "nested task inherits ancestor env and overrides",
			args:    []string{"envgroup", "inner"},
			wantOut: "hello inner g\n",
		},
		{
			name:    "declared arg overrides task env",
			args:    []string{"envarg", "cli"},
			wantOut: "cli\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := execTask(t, tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("runTask() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("runTask() error = %v", err)
			}
			if out != tt.wantOut {
				t.Errorf("runTask() output = %q, want %q", out, tt.wantOut)
			}
		})
	}
}

func TestRunTaskEnvOverridesOS(t *testing.T) {
	t.Setenv("SCOPE", "os")

	out, err := execTask(t, []string{"showenv"})
	if err != nil {
		t.Fatalf("runTask() error = %v", err)
	}
	if want := "hello top\n"; out != want {
		t.Errorf("runTask() output = %q, want %q", out, want)
	}
}
