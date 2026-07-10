package runner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ExitError carries the exit code of a failed command so it can be
// propagated as the process exit code.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("command failed with exit code %d", e.Code)
}

// Run executes a command string with `<shell> -c` in the given
// directory; an empty shell means "sh". args become the shell's
// positional parameters ($1, $2, ..., "$@") with $0 set to "run".
// env is the complete child environment ("name=value" entries, later
// duplicates winning); nil means the current process environment.
func Run(shell, command, dir string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if shell == "" {
		shell = "sh"
	}
	cmd := exec.Command(shell, append([]string{"-c", command, "run"}, args...)...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{Code: exitErr.ExitCode()}
		}
		return err
	}
	return nil
}

// Capture executes a command string with `<shell> -c` in the given
// directory and returns its stdout with trailing newlines trimmed,
// like $(...) command substitution; an empty shell means "sh". env is
// the complete child environment; nil means the current process
// environment. stderr passes through. A failure is a plain error, not
// an ExitError: the captured command's exit code must not masquerade
// as the run command's own.
func Capture(shell, command, dir string, env []string, stderr io.Writer) (string, error) {
	if shell == "" {
		shell = "sh"
	}
	cmd := exec.Command(shell, "-c", command)
	cmd.Dir = dir
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
