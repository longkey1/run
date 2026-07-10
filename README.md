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
    description: Build and deploy
    command: |
      go build -o bin/app
      scp bin/app server:/usr/local/bin/
```

Then:

```sh
run            # list tasks (same as `run list`)
run build      # run the "build" task
run version    # show version information
```

## Task file resolution

`run` looks for a task file in the following order:

1. `$RUN_CONFIG` — explicit path via environment variable
2. `.run.yaml` (or `.run.yml`) in the current directory, then each ancestor directory up to the filesystem root
3. `~/.config/run/run.yaml` (or `run.yml`) — global fallback

## Working directory

- Local task file (`.run.yaml` found by ancestor search): commands run in **the directory containing the task file**, like `make` and `just`. Relative paths in commands stay stable regardless of where you invoke `run`.
- Global task file or `$RUN_CONFIG`: commands run in the current directory.

## Reserved task names

`list`, `ls`, `version`, `help`, and `completion` are subcommands and take precedence over tasks with the same name. Avoid using them as task names.

## Task execution

Commands are executed with `sh -c`. The exit code of the task is propagated as the exit code of `run`, so shell chaining like `run test && run build` works as expected.

## License

MIT
