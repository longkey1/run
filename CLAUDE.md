# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`run` is a CLI runtime built with Cobra: it turns commands defined in YAML files (`.run.yaml`) into a command-line interface and executes them, resolving the command file from the current directory (with ancestor traversal) and falling back to `~/.config/run/run.yaml`.

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
- `cmd/` — Cobra root command. There are no Cobra subcommands: all built-in features are flags (`--list`/`-l`, `--version`, `--completion <shell>`), so bare arguments are always command names
  - `root.go` — `run <command> [subcommand... [args...]]` resolves the argument path through nested commands via rootCmd's `RunE`; no args shows the command list. Path resolution is greedy: names matching a subcommand are path segments, the rest become arguments; a literal `--` (split manually in `runCommand`, since `SetInterspersed(false)` keeps it in `args`) forces the boundary. `applyArgs` validates CLI arguments against a command's `args:` declaration, fills defaults, and builds per-argument environment variables. Environment variables from `env:` declarations are accumulated during path resolution (top-level `Config.Env`, then each matched command's `Command.Env`, inner overrides outer, declared args highest) into a single map, emitted as a sorted deterministic `name=value` list by `envList`; values are literal — run never expands `$VAR` in them. Cobra's default `help`/`completion` subcommands are disabled. Exit codes are propagated via `runner.ExitError` in `Execute()`
  - `list.go` — command listing helpers (`runList`, `listCommands`); nested commands are flattened with space-joined paths, followed by the declared argument signature (`<required>` / `[defaulted]`)
- `internal/config/` — YAML schema (`Config`, `Command`; commands nest via `Command.Commands`), loading/validation (`config.go`), and command file resolution (`finder.go`: `$RUN_CONFIG` → ancestor search for `.run.yaml` → `~/.config/run/run.yaml`). Files referenced via `includes:` (allowed at the top level and on any command) are eagerly merged at load time (`expandIncludes`): the included file's commands land flat in the including scope, name collisions are an error, relative paths resolve against the including file's directory, cycles are detected via an absolute-path chain, and `Includes` is cleared after expansion so the rest of the code only ever sees an inline command tree. An included file's top-level `env:` is pushed down into each command it defines (the command's own `env:` wins), so it never leaks to sibling commands in the including file. `env:` keys are validated (non-empty, no `=`) via `validateEnv`
- `internal/runner/` — executes `run:` strings with `sh -c`; arguments become the shell's positional parameters (`$1`, `"$@"`; `$0` is `run`) and extra env entries are appended after `os.Environ()` (os/exec keeps the last duplicate, so they override the inherited environment). `ExitError` carries the command's exit code
- `internal/version/` — version info injected via ldflags at build time

Key behavior:

- When a local `.run.yaml` is found by ancestor search, commands run in the directory containing the file (like make/just)
- Commands may define `run`, nested `commands`, or both (validated recursively after include expansion). A command with both runs its own `run` string when invoked directly; a group without one lists its subcommands
- Flags must come before the command name (`SetInterspersed(false)`); everything after the first non-flag argument is part of the command path
