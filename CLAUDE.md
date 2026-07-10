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
- `cmd/` — Cobra commands. Each file registers itself to `rootCmd` in `init()`
  - `root.go` — `run <task>` executes a task via rootCmd's `RunE`; no args shows the task list. Task exit codes are propagated via `runner.ExitError` in `Execute()`
  - `list.go` — `list` / `ls` subcommand
  - `version.go` — `version` subcommand
- `internal/config/` — YAML schema (`Config`, `Task`), loading/validation (`config.go`), and task file resolution (`finder.go`: `$RUN_CONFIG` → ancestor search for `.run.yaml` → `~/.config/run/run.yaml`)
- `internal/runner/` — executes commands with `sh -c`; `ExitError` carries the task's exit code
- `internal/version/` — version info injected via ldflags at build time

Key behavior: when a local `.run.yaml` is found by ancestor search, tasks run in the directory containing the file (like make/just). Subcommand names (`list`, `ls`, `version`, `help`, `completion`) shadow tasks with the same name.
