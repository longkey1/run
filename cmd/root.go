package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/longkey1/run/internal/config"
	"github.com/longkey1/run/internal/runner"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "run [task]",
	Short:         "A simple task runner",
	Long:          `run is a simple task runner that executes tasks defined in YAML files (.run.yaml).`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return runList(cmd)
		}
		return runTask(cmd, args[0])
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exitErr *runner.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(1)
	}
}

func loadConfig() (*config.Config, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	path, workDir, err := config.Find(cwd)
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, "", err
	}
	return cfg, workDir, nil
}

func runTask(cmd *cobra.Command, name string) error {
	cfg, workDir, err := loadConfig()
	if err != nil {
		return err
	}
	task, ok := cfg.Tasks[name]
	if !ok {
		return fmt.Errorf("task %q not found", name)
	}
	return runner.Run(task.Command, workDir, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
}
