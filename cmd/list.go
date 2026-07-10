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
			label := full + argSignature(c.Args) + flagSignature(c.Flags)
			if c.Description == "" {
				fmt.Fprintf(w, "  %s\n", label)
			} else {
				fmt.Fprintf(w, "  %s\t- %s\n", label, c.Description)
			}
		}
		writeCommands(w, c.Commands, full)
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

// flagSignature renders declared flags after the argument signature.
// All flags are optional, so every entry is bracketed: " [--name]" for
// bool flags, " [--name <name>]" for value options.
func flagSignature(flags []config.Flag) string {
	var b strings.Builder
	for _, f := range flags {
		if f.IsBool() {
			fmt.Fprintf(&b, " [--%s]", f.Name)
		} else {
			fmt.Fprintf(&b, " [--%s <%s>]", f.Name, f.Name)
		}
	}
	return b.String()
}
