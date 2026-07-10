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
- `cmd/` — Cobra root command. Built-in features live under the single reserved `self` subcommand (`run self list`, `run self version`, `run self completion <shell>`), so every other bare argument is a command name
  - `root.go` — `run <command> [subcommand... [args...]]` resolves the argument path through nested commands via rootCmd's `RunE`; no args shows the command list. Path resolution is greedy: names matching a subcommand are path segments, the rest become arguments; a literal `--` (split manually in `runCommand`, since `SetInterspersed(false)` keeps it in `args`) forces the boundary — tokens after it are always literal, never flags. Processing order: split at `--` → path resolution → `applyFlags` on the pre-`--` remainder → `applyArgs` on positionals + literals → normalized flag tokens appended last. `applyFlags` extracts declared long-form flags (`--name`, `--name=value`, `--name value`; only for commands with a `flags:` declaration — others pass through untouched, unknown `--x` is an error otherwise), builds per-flag env vars (bool `true`/`false`, value options fall back to default or empty; every declared flag always gets an entry), and re-emits recognized flags in declaration order after all positionals so `$1..$n` stay stable and `"$@"` forwards everything (defaults materialize; unset bools and valueless options are omitted). `applyArgs` validates CLI arguments against a command's `args:` declaration, fills defaults, and builds per-argument environment variables. Environment variables from `env:` declarations are accumulated unevaluated (`config.Value`) during path resolution (top-level `Config.Env`, then each matched command's `Command.Env`, inner overrides outer, declared args/flags highest — they cannot collide, validation forbids shared names); `resolveEnv` materializes them only when the command executes: literals pass through, dynamic values run via `runner.Capture` in name order seeing the OS env plus literal entries only (no dynamic-to-dynamic references; overridden dynamic entries never run). Arg/flag defaults (`*config.Value`) resolve lazily through a closure passed to `applyFlags`/`applyArgs` — only when the default is actually used — with the fully resolved env visible, so defaults can reference shared env values. The merged result is emitted as a sorted deterministic `name=value` list by `envList`; plain string values are literal — run never expands `$VAR` in them, `{run: ...}` is the explicit opt-in for computed values. Cobra's default `help`/`completion` subcommands are disabled; since cobra insists on a help command once subcommands exist (and always offers it in shell completion), `selfCmd` doubles as the help command so no extra name is reserved or completed (side effect: `self` is absent from `--help`'s Available Commands, which is why the Long text names it). Exit codes are propagated via `runner.ExitError` in `Execute()`
  - `self.go` — the reserved `self` namespace and its subcommands (`list`, `version`, `completion`); a new built-in feature means a new subcommand here, never a new reserved top-level name
  - `list.go` — command listing helpers (`runList`, `listCommands`); nested commands are flattened with space-joined paths, followed by the declared argument signature (`<required>` / `[defaulted]`) and flag signature (`[--bool]` / `[--name <name>]`)
- `internal/config/` — YAML schema (`Config`, `Command`; commands nest via `Command.Commands`), loading/validation (`config.go`), and command file resolution (`finder.go`: `$RUN_CONFIG` → ancestor search for `.run.yaml` → `~/.config/run/run.yaml`). Files referenced via `includes:` (allowed at the top level and on any command) are eagerly merged at load time (`expandIncludes`): the included file's commands land flat in the including scope, name collisions are an error, relative paths resolve against the including file's directory, cycles are detected via an absolute-path chain, and `Includes` is cleared after expansion so the rest of the code only ever sees an inline command tree. An included file's top-level `env:` is pushed down into each command it defines (the command's own `env:` wins), so it never leaks to sibling commands in the including file. `env:` keys are validated (non-empty, no `=`) via `validateEnv`. `env:` values and arg/flag defaults are `config.Value`: a plain scalar is literal, the `{run: ...}` mapping form is dynamic (a shell command whose stdout, trailing newlines trimmed, becomes the value at execution time); the shape (`run` as the only key, non-empty) is enforced in `Value.UnmarshalYAML`. `flags:` declarations (`Flag`: name/description/type/default; `type: bool` vs value option) are validated via `validateFlags`: require `run:`, names non-empty/unique/no `=`/no leading `-`, no collision with `args:` names (both become env vars), `type` limited to empty/`string`/`bool`, and bool flags may not declare a default (no `--no-name` form exists, so unset must mean false). `Validate` rejects a top-level command named `self` (the reserved built-in namespace); nested commands may use the name
- `internal/runner/` — executes `run:` strings with `sh -c`; arguments become the shell's positional parameters (`$1`, `"$@"`; `$0` is `run`) and extra env entries are appended after `os.Environ()` (os/exec keeps the last duplicate, so they override the inherited environment). `ExitError` carries the command's exit code. `Capture` runs a string the same way but returns its stdout with trailing newlines trimmed (used for dynamic values); its failures are plain errors, never `ExitError`, so a captured command's exit code can't masquerade as the run command's own
- `internal/version/` — version info injected via ldflags at build time

Key behavior:

- When a local `.run.yaml` is found by ancestor search, commands run in the directory containing the file (like make/just)
- Commands may define `run`, nested `commands`, or both (validated recursively after include expansion). A command with both runs its own `run` string when invoked directly; a group without one lists its subcommands
- Flags must come before the command name (`SetInterspersed(false)`); everything after the first non-flag argument is part of the command path
