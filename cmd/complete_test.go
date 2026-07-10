package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

// execComplete runs completeCommands against a temp command file and
// returns the candidates, asserting the directive never falls back to
// file completion.
func execComplete(t *testing.T, args []string, toComplete string) []string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".run.yaml")
	if err := os.WriteFile(path, []byte(testCommands), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUN_CONFIG", path)

	names, directive := completeCommands(rootCmd, args, toComplete)
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("completeCommands() directive = %v, want NoFileComp", directive)
	}
	return names
}

func TestCompleteCommands(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		toComplete string
		want       []string
	}{
		{
			name: "nested command names",
			args: []string{"db"},
			want: []string{"migrate"},
		},
		{
			name:       "declared options plus implicit --help",
			args:       []string{"optcmd"},
			toComplete: "--",
			want:       []string{"--force", "--from", "--label", "--help\tshow this help"},
		},
		{
			name:       "options filtered by prefix",
			args:       []string{"optcmd"},
			toComplete: "--f",
			want:       []string{"--force", "--from"},
		},
		{
			name:       "option descriptions included",
			args:       []string{"described"},
			toComplete: "--",
			want:       []string{"--force\tskip confirmation", "--from", "--help\tshow this help"},
		},
		{
			name:       "options still offered after a positional",
			args:       []string{"optcmd", "t"},
			toComplete: "--",
			want:       []string{"--force", "--from", "--label", "--help\tshow this help"},
		},
		{
			name:       "used bool option filtered out",
			args:       []string{"optcmd", "--force"},
			toComplete: "--",
			want:       []string{"--from", "--label", "--help\tshow this help"},
		},
		{
			name:       "used equals-form option filtered out",
			args:       []string{"optcmd", "--from=a"},
			toComplete: "--",
			want:       []string{"--force", "--label", "--help\tshow this help"},
		},
		{
			name: "value option pending value gets no candidates",
			args: []string{"optcmd", "--from"},
		},
		{
			name:       "pending value looking like an option gets no candidates",
			args:       []string{"optcmd", "t", "--label"},
			toComplete: "--f",
		},
		{
			name:       "value part of --name= gets no candidates",
			args:       []string{"optcmd"},
			toComplete: "--from=",
		},
		{
			name:       "nothing after a literal --",
			args:       []string{"optcmd", "--"},
			toComplete: "--f",
		},
		{
			name: "positional position gets no candidates",
			args: []string{"db", "foo"},
		},
		{
			name:       "declared help option is not duplicated",
			args:       []string{"helpopt"},
			toComplete: "--",
			want:       []string{"--help"},
		},
		{
			name:       "unknown command gets no option candidates",
			args:       []string{"nope"},
			toComplete: "--",
		},
		{
			name:       "command without declared options still offers --help",
			args:       []string{"echo"},
			toComplete: "--",
			want:       []string{"--help\tshow this help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execComplete(t, tt.args, tt.toComplete)
			if !slices.Equal(got, tt.want) {
				t.Errorf("completeCommands(%v, %q) = %q, want %q", tt.args, tt.toComplete, got, tt.want)
			}
		})
	}
}

// TestCompleteCommandsTopLevel checks top-level name completion, which
// iterates a map: order is not deterministic, so assert membership.
func TestCompleteCommandsTopLevel(t *testing.T) {
	got := execComplete(t, nil, "")
	for _, want := range []string{"deploy", "group", "described\tDeploy the app"} {
		if !slices.Contains(got, want) {
			t.Errorf("completeCommands(nil, \"\") = %q, missing %q", got, want)
		}
	}
}
