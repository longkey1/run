package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/longkey1/run/internal/config"
	"github.com/spf13/cobra"
)

func runList(cmd *cobra.Command) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	return listTasks(cmd.OutOrStdout(), cfg.Tasks, "")
}

// listTasks writes all runnable tasks (those with a command) under the
// given prefix, one per line with the full space-joined path.
func listTasks(out io.Writer, tasks map[string]config.Task, prefix string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	writeTasks(w, tasks, prefix)
	return w.Flush()
}

func writeTasks(w io.Writer, tasks map[string]config.Task, prefix string) {
	names := make([]string, 0, len(tasks))
	for name := range tasks {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		task := tasks[name]
		full := name
		if prefix != "" {
			full = prefix + " " + name
		}
		if task.Command != "" {
			label := full + argSignature(task.Args)
			if task.Description == "" {
				fmt.Fprintf(w, "  %s\n", label)
			} else {
				fmt.Fprintf(w, "  %s\t- %s\n", label, task.Description)
			}
		}
		writeTasks(w, task.Tasks, full)
	}
}

// argSignature renders declared args as " <name>" for required
// arguments and " [name]" for arguments with a default.
func argSignature(args []config.Arg) string {
	var b strings.Builder
	for _, a := range args {
		if a.Default != nil {
			fmt.Fprintf(&b, " [%s]", a.Name)
		} else {
			fmt.Fprintf(&b, " <%s>", a.Name)
		}
	}
	return b.String()
}
