package cmd

import (
	"fmt"
	"io"

	"github.com/longkey1/run/internal/config"
)

// helpRow is one "  label  detail" line in a help section. Rows with a
// detail are aligned on a shared column; rows without one are printed
// bare so no line carries trailing padding.
type helpRow struct {
	label  string
	detail string
}

// commandHelp renders declared help for a resolved command: its
// description, usage lines built from the same signatures as the list
// output, and Arguments/Options/Commands sections. Dynamic defaults
// are shown as "dynamic" and never executed, matching self list and
// shell completion.
func commandHelp(out io.Writer, c config.Command, name string) error {
	if c.Description != "" {
		fmt.Fprintf(out, "%s\n\n", c.Description)
	}
	fmt.Fprintln(out, "Usage:")
	if c.Run != "" {
		fmt.Fprintf(out, "  run %s%s%s\n", name, argSignature(c.Args), flagSignature(c.Flags))
	}
	if len(c.Commands) > 0 {
		fmt.Fprintf(out, "  run %s <command>\n", name)
	}
	if len(c.Args) > 0 {
		fmt.Fprintln(out, "\nArguments:")
		writeHelpRows(out, argHelpRows(c.Args))
	}
	// Groups cannot declare flags, so an Options section holding only
	// --help would be noise there; runnable commands always get one.
	if c.Run != "" {
		fmt.Fprintln(out, "\nOptions:")
		writeHelpRows(out, flagHelpRows(c.Flags))
	}
	if len(c.Commands) > 0 {
		fmt.Fprintln(out, "\nCommands:")
		return listCommands(out, c.Commands, name)
	}
	return nil
}

// argHelpRows renders one row per declared argument, mirroring the
// usage signature: <name> for required arguments, [name] for arguments
// with a default.
func argHelpRows(args []config.Arg) []helpRow {
	rows := make([]helpRow, 0, len(args))
	for _, a := range args {
		label := "<" + a.Name + ">"
		if a.Default != nil {
			label = "[" + a.Name + "]"
		}
		rows = append(rows, helpRow{label, detailText(a.Description, a.Default)})
	}
	return rows
}

// flagHelpRows renders one row per declared flag, plus a trailing
// --help entry unless the command declares a flag named help itself
// (that declaration also disables --help interception).
func flagHelpRows(flags []config.Flag) []helpRow {
	rows := make([]helpRow, 0, len(flags)+1)
	for _, f := range flags {
		label := "--" + f.Name
		if !f.IsBool() {
			label += " <" + f.Name + ">"
		}
		rows = append(rows, helpRow{label, detailText(f.Description, f.Default)})
	}
	if !hasFlag(flags, "help") {
		rows = append(rows, helpRow{"--help", "show this help"})
	}
	return rows
}

func writeHelpRows(out io.Writer, rows []helpRow) {
	width := 0
	for _, r := range rows {
		if r.detail != "" {
			width = max(width, len(r.label))
		}
	}
	for _, r := range rows {
		if r.detail == "" {
			fmt.Fprintf(out, "  %s\n", r.label)
			continue
		}
		fmt.Fprintf(out, "  %-*s  %s\n", width, r.label, r.detail)
	}
}

// detailText joins a declared description with its default marker.
func detailText(description string, def *config.Value) string {
	label := defaultLabel(def)
	switch {
	case label == "":
		return description
	case description == "":
		return label
	default:
		return description + " " + label
	}
}

// defaultLabel renders a declared default for help output. Dynamic
// defaults ({run: ...}) are labeled, never executed — help must not
// run any user command.
func defaultLabel(d *config.Value) string {
	switch {
	case d == nil:
		return ""
	case d.IsDynamic():
		return "(default: dynamic)"
	case d.Literal == "":
		return `(default: "")`
	default:
		return "(default: " + d.Literal + ")"
	}
}

// declaresFlag reports whether the command declares a flag with the
// given name.
func declaresFlag(c config.Command, name string) bool {
	return hasFlag(c.Flags, name)
}

func hasFlag(flags []config.Flag, name string) bool {
	for _, f := range flags {
		if f.Name == name {
			return true
		}
	}
	return false
}
