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
	sources, err := loadSources()
	if err != nil {
		return err
	}
	return listCommands(cmd.OutOrStdout(), mergedCommands(sources), "")
}

// listCommands writes all runnable commands (those with a run string)
// under the given prefix, one per line with the full space-joined path.
func listCommands(out io.Writer, cmds map[string]config.Command, prefix string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	writeCommands(w, cmds, prefix)
	return w.Flush()
}

func writeCommands(w io.Writer, cmds map[string]config.Command, prefix string) {
	names := make([]string, 0, len(cmds))
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		c := cmds[name]
		full := name
		if prefix != "" {
			full = prefix + " " + name
		}
		if c.Run != "" {
			label := full + argumentSignature(c.Arguments) + optionSignature(c.Options)
			if c.Description == "" {
				fmt.Fprintf(w, "  %s\n", label)
			} else {
				fmt.Fprintf(w, "  %s\t- %s\n", label, c.Description)
			}
		}
		writeCommands(w, c.Commands, full)
	}
}

// argumentSignature renders declared arguments as " <name>" for
// required arguments and " [name]" for arguments with a default.
func argumentSignature(args []config.Argument) string {
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

// optionSignature renders declared options after the argument
// signature. All options are optional, so every entry is bracketed:
// " [--name]" for bool options, " [--name <name>]" for value options.
func optionSignature(options []config.Option) string {
	var b strings.Builder
	for _, o := range options {
		if o.IsBool() {
			fmt.Fprintf(&b, " [--%s]", o.Name)
		} else {
			fmt.Fprintf(&b, " [--%s <%s>]", o.Name, o.Name)
		}
	}
	return b.String()
}
