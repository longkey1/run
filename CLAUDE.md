# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`run` is a simple task runner CLI built with Cobra. It executes tasks defined in YAML files (`.run.yaml`), resolving the task file from the current directory (with ancestor traversal) and falling back to `~/.config/run/run.yaml`.

## Build Commands

```sh
make build   # Build binary to ./bin/run (with version ldflags)
make test    # Run tests
make fmt     # Format code
make vet     # Vet code
make tidy    # Tidy dependencies
make clean   # Remove build artifacts
```

Binary name is read from `.product_name`.

## Release

```sh
make release type=patch|minor|major            # dry run (default)
make release type=patch dryrun=false           # create and push tag
make re-release [tag=vX.Y.Z] dryrun=false      # re-release an existing tag
```

Pushing a `v*` tag triggers `.github/workflows/gorelease.yml`, which builds multi-platform binaries with GoReleaser and uploads them to GitHub Releases.

## Architecture

- `main.go` — entry point, calls `cmd.Execute()`
- `cmd/` — Cobra root command. There are no subcommands: all built-in features are flags (`--list`/`-l`, `--version`, `--completion <shell>`), so bare arguments are always task names
  - `root.go` — `run <task> [subtask... [args...]]` resolves the argument path through nested tasks via rootCmd's `RunE`; no args shows the task list. Path resolution is greedy: names matching a subtask are path segments, the rest become task arguments; a literal `--` (split manually in `runTask`, since `SetInterspersed(false)` keeps it in `args`) forces the boundary. `applyArgs` validates CLI arguments against a task's `args:` declaration, fills defaults, and builds per-argument environment variables. Cobra's default `help`/`completion` subcommands are disabled. Task exit codes are propagated via `runner.ExitError` in `Execute()`
  - `list.go` — task listing helpers (`runList`, `listTasks`); nested tasks are flattened with space-joined paths, followed by the declared argument signature (`<required>` / `[defaulted]`)
- `internal/config/` — YAML schema (`Config`, `Task`; tasks nest via `Task.Tasks`), loading/validation (`config.go`), and task file resolution (`finder.go`: `$RUN_CONFIG` → ancestor search for `.run.yaml` → `~/.config/run/run.yaml`). External task files referenced via `Task.File` are eagerly expanded at load time (`expandTasks`): relative paths resolve against the referencing file's directory, cycles are detected via an absolute-path chain, and `File` is cleared after expansion so the rest of the code only ever sees an inline task tree
- `internal/runner/` — executes commands with `sh -c`; task arguments become the shell's positional parameters (`$1`, `"$@"`; `$0` is `run`) and declared arguments are also appended to the environment. `ExitError` carries the task's exit code
- `internal/version/` — version info injected via ldflags at build time

Key behavior:

- When a local `.run.yaml` is found by ancestor search, tasks run in the directory containing the file (like make/just)
- Tasks may define `command`, nested `tasks`, or both (validated recursively). A task with both runs its own command when invoked directly; a group without a command lists its subtasks
- Flags must come before the task name (`SetInterspersed(false)`); everything after the first non-flag argument is part of the task path
