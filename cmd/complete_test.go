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
			name:       "declared flags plus implicit --help",
			args:       []string{"flagcmd"},
			toComplete: "--",
			want:       []string{"--force", "--from", "--label", "--help\tshow this help"},
		},
		{
			name:       "flags filtered by prefix",
			args:       []string{"flagcmd"},
			toComplete: "--f",
			want:       []string{"--force", "--from"},
		},
		{
			name:       "flag descriptions included",
			args:       []string{"described"},
			toComplete: "--",
			want:       []string{"--force\tskip confirmation", "--from", "--help\tshow this help"},
		},
		{
			name:       "flags still offered after a positional",
			args:       []string{"flagcmd", "t"},
			toComplete: "--",
			want:       []string{"--force", "--from", "--label", "--help\tshow this help"},
		},
		{
			name:       "used bool flag filtered out",
			args:       []string{"flagcmd", "--force"},
			toComplete: "--",
			want:       []string{"--from", "--label", "--help\tshow this help"},
		},
		{
			name:       "used equals-form flag filtered out",
			args:       []string{"flagcmd", "--from=a"},
			toComplete: "--",
			want:       []string{"--force", "--label", "--help\tshow this help"},
		},
		{
			name: "value flag pending value gets no candidates",
			args: []string{"flagcmd", "--from"},
		},
		{
			name:       "pending value looking like a flag gets no candidates",
			args:       []string{"flagcmd", "t", "--label"},
			toComplete: "--f",
		},
		{
			name:       "value part of --name= gets no candidates",
			args:       []string{"flagcmd"},
			toComplete: "--from=",
		},
		{
			name:       "nothing after a literal --",
			args:       []string{"flagcmd", "--"},
			toComplete: "--f",
		},
		{
			name: "positional position gets no candidates",
			args: []string{"db", "foo"},
		},
		{
			name:       "declared help flag is not duplicated",
			args:       []string{"helpflag"},
			toComplete: "--",
			want:       []string{"--help"},
		},
		{
			name:       "unknown command gets no flag candidates",
			args:       []string{"nope"},
			toComplete: "--",
		},
		{
			name:       "command without declared flags still offers --help",
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
