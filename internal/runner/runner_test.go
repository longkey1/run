package runner

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeShell writes an executable stub shell that prints each of its
// arguments on its own line, so tests can assert exactly how the
// runner invokes the configured shell.
func fakeShell(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fakeshell")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		command    string
		args       []string
		env        []string
		wantStdout string
		wantCode   int // 0 means success
	}{
		{
			name:       "stdout is forwarded",
			command:    "echo hello",
			wantStdout: "hello\n",
		},
		{
			name:     "exit code is propagated",
			command:  "exit 3",
			wantCode: 3,
		},
		{
			name:       "multi-line command",
			command:    "echo one\necho two",
			wantStdout: "one\ntwo\n",
		},
		{
			name:       "args become positional parameters",
			command:    `echo "$1-$2"`,
			args:       []string{"a", "b"},
			wantStdout: "a-b\n",
		},
		{
			name:       "all args via $@",
			command:    `printf '%s\n' "$@"`,
			args:       []string{"a", "b c", "d"},
			wantStdout: "a\nb c\nd\n",
		},
		{
			name:       "$0 is run",
			command:    `echo "$0"`,
			wantStdout: "run\n",
		},
		{
			name:       "extra env is visible",
			command:    `echo "$env-$region"`,
			env:        []string{"env=prod", "region=jp"},
			wantStdout: "prod-jp\n",
		},
		{
			// os/exec keeps the last entry for duplicate keys (Go 1.19+),
			// so extra env appended after os.Environ() wins.
			name:       "extra env overrides inherited environ",
			command:    `echo "$HOME"`,
			env:        []string{"HOME=/override"},
			wantStdout: "/override\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout bytes.Buffer
			err := Run("", tt.command, t.TempDir(), tt.args, tt.env, nil, &stdout, io.Discard)

			if tt.wantCode == 0 {
				if err != nil {
					t.Fatalf("Run() error = %v, want nil", err)
				}
			} else {
				var exitErr *ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("Run() error = %v, want *ExitError", err)
				}
				if exitErr.Code != tt.wantCode {
					t.Errorf("Run() exit code = %d, want %d", exitErr.Code, tt.wantCode)
				}
			}
			if got := stdout.String(); got != tt.wantStdout {
				t.Errorf("Run() stdout = %q, want %q", got, tt.wantStdout)
			}
		})
	}
}

func TestRunWorkDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// t.TempDir may contain symlinks (e.g. /var -> /private/var on macOS),
	// so use `pwd -P` and compare physical paths.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := Run("", "pwd -P", dir, nil, nil, nil, &stdout, io.Discard); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Errorf("Run() pwd = %q, want %q", got, want)
	}
}

func TestRunCustomShell(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	if err := Run(fakeShell(t), "echo hi", t.TempDir(), []string{"a"}, nil, nil, &stdout, io.Discard); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "-c\necho hi\nrun\na\n"
	if got := stdout.String(); got != want {
		t.Errorf("Run() stdout = %q, want %q", got, want)
	}
}

func TestCaptureCustomShell(t *testing.T) {
	t.Parallel()

	out, err := Capture(fakeShell(t), "echo hi", t.TempDir(), nil, io.Discard)
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if want := "-c\necho hi"; out != want {
		t.Errorf("Capture() = %q, want %q", out, want)
	}
}

func TestRunStderr(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := Run("", "echo oops >&2", t.TempDir(), nil, nil, nil, &stdout, &stderr); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := stderr.String(); got != "oops\n" {
		t.Errorf("Run() stderr = %q, want %q", got, "oops\n")
	}
	if got := stdout.String(); got != "" {
		t.Errorf("Run() stdout = %q, want empty", got)
	}
}
