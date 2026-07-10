package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ExitError carries the exit code of a failed command so it can be
// propagated as the process exit code.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("command failed with exit code %d", e.Code)
}

// Run executes a command string with `sh -c` in the given directory.
// args become the shell's positional parameters ($1, $2, ..., "$@")
// with $0 set to "run". extraEnv entries ("name=value") are appended
// to the current environment.
func Run(command, dir string, args, extraEnv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command("sh", append([]string{"-c", command, "run"}, args...)...)
	cmd.Dir = dir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
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
