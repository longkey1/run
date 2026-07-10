# run

A simple task runner that executes tasks defined in YAML files.

## Installation

Download a binary from [GitHub Releases](https://github.com/longkey1/run/releases), or build from source:

```sh
go install github.com/longkey1/run@latest
```

Note: `run` may collide with shell builtins or functions in some environments. Use `command run` or check `which run` if the wrong command is invoked.

## Usage

Define tasks in `.run.yaml`:

```yaml
tasks:
  build:
    description: Build the project
    command: go build -o bin/app
  test:
    description: Run tests
    command: go test ./...
  deploy:
    description: Deploy the app
    tasks:
      staging:
        command: ./deploy.sh staging
      production:
        command: ./deploy.sh production
```

Then:

```sh
run                    # list tasks
run build              # run the "build" task
run deploy staging     # run a nested task
```

## Nested tasks

Tasks can be nested with `tasks:` to form subcommands:

- `run deploy staging` walks the task tree by argument path.
- A task may define `command`, nested `tasks`, or both. With both, `run deploy` runs its own command; without a command, `run deploy` lists its subtasks.
- `run` / `run --list` shows runnable tasks flattened with their full path (e.g. `deploy staging`).

## Built-in flags

All of run's own features are flags, so bare arguments are always task names and there are no reserved task names:

```sh
run --list             # list tasks (same as plain `run`), also -l
run --version          # show version information
run --completion zsh   # generate shell completion (bash|zsh|fish|powershell)
run --help             # show help
```

Flags must come before the task name; everything after the first non-flag argument is treated as part of the task path.

## Task file resolution

`run` looks for a task file in the following order:

1. `$RUN_CONFIG` — explicit path via environment variable
2. `.run.yaml` (or `.run.yml`) in the current directory, then each ancestor directory up to the filesystem root
3. `~/.config/run/run.yaml` (or `run.yml`) — global fallback

## Working directory

- Local task file (`.run.yaml` found by ancestor search): commands run in **the directory containing the task file**, like `make` and `just`. Relative paths in commands stay stable regardless of where you invoke `run`.
- Global task file or `$RUN_CONFIG`: commands run in the current directory.

## Task execution

Commands are executed with `sh -c`. The exit code of the task is propagated as the exit code of `run`, so shell chaining like `run test && run build` works as expected.

## License

MIT
