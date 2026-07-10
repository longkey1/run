package runner

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// ExitError carries the exit code of a failed task so it can be
// propagated as the process exit code.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("task failed with exit code %d", e.Code)
}

// Run executes a command string with `sh -c` in the given directory.
func Run(command, dir string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
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
