# run

A CLI runtime: define commands in YAML, and `run` turns them into a command-line interface with subcommands, arguments, environment variables, and shell completion.

## Installation

Download a binary from [GitHub Releases](https://github.com/longkey1/run/releases), or build from source:

```sh
go install github.com/longkey1/run@latest
```

Note: `run` may collide with shell builtins or functions in some environments. Use `command run` or check `which run` if the wrong command is invoked.

## Usage

Define commands in `.run.yaml`:

```yaml
commands:
  build:
    description: Build the project
    run: go build -o bin/app
  test:
    description: Run tests
    run: go test ./...
  deploy:
    description: Deploy the app
    commands:
      staging:
        run: ./deploy.sh staging
      production:
        run: ./deploy.sh production
```

Then:

```sh
run                    # list commands
run build              # run the "build" command
run deploy staging     # run a nested command
```

## Nested commands

Commands can be nested with `commands:` to form subcommands:

- `run deploy staging` walks the command tree by argument path.
- A command may define `run`, nested `commands`, or both. With both, `run deploy` runs its own `run` string; without one, `run deploy` lists its subcommands.
- `run` / `run self list` shows runnable commands flattened with their full path (e.g. `deploy staging`).

## Arguments

Arguments after the command name are passed to the `run` string as shell positional parameters:

```yaml
commands:
  greet:
    run: echo "hello $1"
  k:
    description: kubectl wrapper
    run: kubectl "$@"
```

```sh
run greet world        # hello world
run k get pods         # kubectl get pods ("$@" passes everything through)
```

- `$1`, `$2`, ... reference individual arguments; `"$@"` expands to all of them (quoting is handled by the shell, so spaces in arguments are preserved).
- The command path is resolved greedily: names matching a subcommand are path segments, and the rest become arguments. Use `--` to force the boundary when an argument collides with a subcommand name (`run db -- migrate` passes `migrate` as `$1` instead of running the subcommand).

### Declared arguments

A command can declare its arguments with `args:` to require them, give them defaults, and reference them by name:

```yaml
commands:
  deploy:
    description: Deploy the app
    args:
      - name: env
        description: target environment
      - name: region
        default: us-east-1
    run: ./deploy.sh "$env" "$region"
```

```sh
run deploy prod jp     # ./deploy.sh prod jp
run deploy prod        # ./deploy.sh prod us-east-1 (default applied)
run deploy             # error: command "deploy": missing required argument "env"
```

- CLI arguments map to declared args in order. Missing trailing arguments fall back to their `default`; a missing argument without a default is an error.
- Each declared argument is available both positionally (`$1`, ...) and as an environment variable named after it (`$env`, `$region`). Defaults are applied to both.
- Arguments beyond the declaration are still passed through (`"$@"` includes them), so declarations and pass-through wrappers compose.
- Commands without `args:` accept any number of arguments without validation.
- `run self list` shows the signature: `deploy <env> [region]` (`<...>` required, `[...]` has a default).

## Environment variables

Environment variables can be declared with `env:`, at the top level of the command file and/or per command:

```yaml
env:
  APP_ENV: development
commands:
  build:
    run: go build -o bin/app
  deploy:
    env:
      APP_ENV: production
    commands:
      staging:
        env:
          APP_ENV: staging
        run: ./deploy.sh
      production:
        run: ./deploy.sh
```

`run build` sees `APP_ENV=development`, `run deploy production` sees `APP_ENV=production`, and `run deploy staging` sees `APP_ENV=staging`.

- Top-level `env:` applies to every command in the file; a command's `env:` applies to the command and its subcommands.
- Precedence (lowest to highest): inherited OS environment < top-level `env` < ancestor command `env` (outer to inner) < the resolved command's `env` < declared-argument variables.
- Values are literal strings — `run` performs no `$VAR`/`${VAR}` expansion in them. Variable references written in `run:` are still expanded by the shell at execution time, so `run: echo "$APP_ENV"` works as expected.

## Includes

Commands can be split across files with `includes:`. The included file's commands are merged flat into the including scope — at the top level or inside a command:

```yaml
# .run.yaml
includes:
  - ./common.yaml        # lint, fmt become top-level commands
commands:
  build:
    run: go build -o bin/app
  deploy:
    description: Deploy commands
    includes:
      - ./deploy.yaml    # staging, production become subcommands of deploy
```

```yaml
# deploy.yaml
commands:
  staging:
    run: ./deploy.sh staging
  production:
    run: ./deploy.sh production
```

Then `run lint` and `run deploy staging` work as if the commands were defined inline.

- An included file uses the same schema as `.run.yaml` (`env`, `includes`, `commands`), and may itself include further files.
- A name collision — an included command with the same name as a local command or one from an earlier include in the same scope — is an error.
- An included file's top-level `env:` applies to the commands it defines (and their subcommands), not to other commands in the including file. A command's own `env:` wins over its file's top-level `env:` on conflict.
- Relative paths resolve against the directory of the including file. Absolute paths are allowed; `~` is not expanded.
- `includes` can be combined freely with `run` and inline `commands` in the same command.
- Includes only split up definitions: commands still run in the root command file's directory, and `run self list` and shell completion cover included commands like inline ones.
- Circular includes are detected and reported as an error.

## Built-in commands

All of run's own features live under the single reserved name `self`, so every other bare argument is a user-defined command name:

```sh
run self list              # list commands (same as plain `run`)
run self version           # show version information
run self completion zsh    # generate shell completion (bash|zsh|fish|powershell)
run --help                 # show help
```

`self` is the only reserved name: a top-level command named `self` is a configuration error. Nested commands may still use the name freely (`run deploy self` works).

Flags must come before the command name; everything after the first non-flag argument is treated as part of the command path.

## Command file resolution

`run` looks for a command file in the following order:

1. `$RUN_CONFIG` — explicit path via environment variable
2. `.run.yaml` (or `.run.yml`) in the current directory, then each ancestor directory up to the filesystem root
3. `~/.config/run/run.yaml` (or `run.yml`) — global fallback

## Working directory

- Local command file (`.run.yaml` found by ancestor search): commands run in **the directory containing the file**, like `make` and `just`. Relative paths in `run:` strings stay stable regardless of where you invoke `run`.
- Global command file or `$RUN_CONFIG`: commands run in the current directory.

## Execution

`run:` strings are executed with `sh -c`, with arguments passed as positional parameters (`$0` is `run`). The exit code of the command is propagated as the exit code of `run`, so shell chaining like `run test && run build` works as expected.

## License

MIT
