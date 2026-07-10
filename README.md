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

## Task arguments

Arguments after the task name are passed to the command as shell positional parameters:

```yaml
tasks:
  greet:
    command: echo "hello $1"
  k:
    description: kubectl wrapper
    command: kubectl "$@"
```

```sh
run greet world        # hello world
run k get pods         # kubectl get pods ("$@" passes everything through)
```

- `$1`, `$2`, ... reference individual arguments; `"$@"` expands to all of them (quoting is handled by the shell, so spaces in arguments are preserved).
- The task path is resolved greedily: names matching a subtask are path segments, and the rest become arguments. Use `--` to force the boundary when an argument collides with a subtask name (`run db -- migrate` passes `migrate` as `$1` instead of running the subtask).

### Declared arguments

A task can declare its arguments with `args:` to require them, give them defaults, and reference them by name:

```yaml
tasks:
  deploy:
    description: Deploy the app
    args:
      - name: env
        description: target environment
      - name: region
        default: us-east-1
    command: ./deploy.sh "$env" "$region"
```

```sh
run deploy prod jp     # ./deploy.sh prod jp
run deploy prod        # ./deploy.sh prod us-east-1 (default applied)
run deploy             # error: task "deploy": missing required argument "env"
```

- CLI arguments map to declared args in order. Missing trailing arguments fall back to their `default`; a missing argument without a default is an error.
- Each declared argument is available both positionally (`$1`, ...) and as an environment variable named after it (`$env`, `$region`). Defaults are applied to both.
- Arguments beyond the declaration are still passed through (`"$@"` includes them), so declarations and pass-through wrappers compose.
- Tasks without `args:` accept any number of arguments without validation.
- `--list` shows the signature: `deploy <env> [region]` (`<...>` required, `[...]` has a default).

## Environment variables

Environment variables for commands can be declared with `env:`, at the top level of the task file and/or per task:

```yaml
env:
  APP_ENV: development
tasks:
  build:
    command: go build -o bin/app
  deploy:
    env:
      APP_ENV: production
    tasks:
      staging:
        env:
          APP_ENV: staging
        command: ./deploy.sh
      production:
        command: ./deploy.sh
```

`run build` sees `APP_ENV=development`, `run deploy production` sees `APP_ENV=production`, and `run deploy staging` sees `APP_ENV=staging`.

- Top-level `env:` applies to every task in the file; a task's `env:` applies to the task and its subtasks.
- Precedence (lowest to highest): inherited OS environment < top-level `env` < ancestor task `env` (outer to inner) < the resolved task's `env` < declared-argument variables.
- Values are literal strings — `run` performs no `$VAR`/`${VAR}` expansion in them. Variable references written in `command:` are still expanded by the shell at execution time, so `command: echo "$APP_ENV"` works as expected.

## External task files

A task's subtasks can be defined in a separate file with `file:`:

```yaml
# .run.yaml
tasks:
  deploy:
    description: Deploy tasks
    file: ./deploy.run.yaml
```

```yaml
# deploy.run.yaml
tasks:
  staging:
    command: ./deploy.sh staging
  production:
    command: ./deploy.sh production
```

Then `run deploy staging` works as if the tasks were defined inline.

- The external file uses the same schema as `.run.yaml` (a top-level `tasks:` map), and may itself reference further files.
- An external file may define its own top-level `env:`; it is merged into the referencing task's `env` and applies to all tasks in the file (the file's values win over the referencing task's on conflict).
- Relative paths resolve against the directory of the referencing file. Absolute paths are allowed; `~` is not expanded.
- `file` can be combined with `command` (like `tasks` + `command`), but not with inline `tasks`.
- External files only split up definitions: commands still run in the root task file's directory, and `--list` and shell completion include external tasks like inline ones.
- Circular references are detected and reported as an error.

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

Commands are executed with `sh -c`, with task arguments passed as positional parameters (`$0` is `run`). The exit code of the task is propagated as the exit code of `run`, so shell chaining like `run test && run build` works as expected.

## License

MIT
