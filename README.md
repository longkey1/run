# run

A CLI runtime: define commands in YAML, and `run` turns them into a command-line interface with subcommands, arguments, flags, environment variables, and shell completion.

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
- A `default` may also be computed by a shell command — see [Dynamic values](#dynamic-values).
- `run self list` shows the signature: `deploy <env> [region]` (`<...>` required, `[...]` has a default).

### Declared flags

A command can declare long-form flags with `flags:` — boolean flags (`type: bool`) and value options (the default):

```yaml
commands:
  deploy:
    description: Deploy the app
    args:
      - name: env
    flags:
      - name: force
        type: bool
        description: skip confirmation
      - name: from
        default: "2026-01-01"
    run: ./deploy.sh
```

```sh
run deploy prod --force                # $force=true
run deploy --force prod                # flags and arguments may be interleaved
run deploy prod --from 2026-04-01      # $from=2026-04-01
run deploy prod --from=2026-04-01      # equals form works too
run deploy prod --typo                 # error: unknown flag --typo
```

- Every declared flag becomes an environment variable named after it. Bool flags are `true` when passed and `false` otherwise; value options get the given value, their `default`, or the empty string. All flags are optional — there are no required flags.
- Flags are also re-appended to the positional parameters *after* all positionals, normalized to `--name value` / `--name` in declaration order. `$1..$n` are always the plain arguments, and `"$@"` forwards everything — flags included — to a wrapped command. Defaults materialize there too (like `args:` defaults); a bool that wasn't passed and a value option with no value are omitted.
- Only long form is supported. Single-dash tokens (`-x`) are always ordinary arguments. A repeated flag: the last one wins. A space-form value is taken literally even if it starts with `--` (use `--name=value` when the value looks like a flag).
- Unknown `--x` is an error only for commands that declare `flags:`; commands without `flags:` pass everything through untouched (except `--help` — see [Help](#help)). Tokens after `--` are always literal arguments, never flags.
- A value option's `default` may also be computed by a shell command — see [Dynamic values](#dynamic-values).
- `run self list` shows flags after the argument signature: `deploy <env> [--force] [--from <from>]`.
- Shell completion suggests declared flags with their descriptions: `run deploy --<TAB>` offers `--force`, `--from`, and `--help`. Flags already on the command line are not suggested again, and nothing is suggested in a value position or after `--`.

## Help

`run <command> --help` shows a command's declared help, built from its `description`, `args:`, and `flags:`:

```sh
$ run deploy --help
Deploy the app

Usage:
  run deploy <env> [--force] [--from <from>]

Arguments:
  <env>  target environment

Options:
  --force        skip confirmation
  --from <from>  (default: 2026-01-01)
  --help         show this help
```

- A `--help` anywhere before `--` shows help instead of running the command — including for commands without `flags:` that otherwise pass flags through. To forward a literal `--help` to a wrapped command, put it after `--`: `run k -- --help`.
- A command that declares its own flag named `help` opts out — `--help` is then parsed like any other declared flag.
- A group command (no `run:`) lists its subcommands.
- Dynamic defaults are shown as `(default: dynamic)`; help never executes any shell command.

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
- Precedence (lowest to highest): inherited OS environment < top-level `env` < ancestor command `env` (outer to inner) < the resolved command's `env` < declared-argument and declared-flag variables.
- Values are literal strings — `run` performs no `$VAR`/`${VAR}` expansion in them. Variable references written in `run:` are still expanded by the shell at execution time, so `run: echo "$APP_ENV"` works as expected. To compute a value with a shell command, use the explicit `{run: ...}` form — see [Dynamic values](#dynamic-values).

## Dynamic values

`env:` values and `args:`/`flags:` defaults are literal strings by default. The mapping form `{run: ...}` opts a single value into dynamic evaluation: the shell command's stdout (with trailing newlines trimmed, like `$(...)` substitution) becomes the value.

```yaml
env:
  TODAY:
    run: date +%F              # computed once per invocation
commands:
  report:
    args:
      - name: date
        default:
          run: echo "$TODAY"   # defaults can reference resolved env values
    run: echo "report for $1"
```

```sh
run report                # report for 2026-07-10
run report 2020-01-01     # report for 2020-01-01 (the default's command does not run)
```

- Evaluation happens only when a command is executed — never for `run self list`, `--help`, or shell completion — and only for the values that invocation actually uses: an overridden dynamic `env` entry and an unused default are never run.
- Dynamic `env` values see the OS environment plus the literal `env` entries; they cannot reference other dynamic `env` values. Defaults are resolved after `env`, so they see all of it — define a shared value like `TODAY` once and reference it from any default.
- Dynamic values run with the same shell as the command itself (`sh -c` unless overridden via `shell:` — see [Shell](#shell)) in the same directory (the directory containing the command file). A non-zero exit aborts the invocation with an error.
- Bool flags still may not declare a default, dynamic or otherwise.

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

- An included file uses the same schema as `.run.yaml` (`shell`, `env`, `includes`, `commands`), and may itself include further files.
- A name collision — an included command with the same name as a local command or one from an earlier include in the same scope — is an error.
- An included file's top-level `env:` applies to the commands it defines (and their subcommands), not to other commands in the including file. A command's own `env:` wins over its file's top-level `env:` on conflict.
- An included file's top-level `shell:` works the same way: it applies to the commands the file defines, and a command's own `shell:` wins.
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
run --help                 # show run's own help
run <command> --help       # show a command's declared help
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

`run:` strings are executed with `sh -c` by default, with arguments passed as positional parameters (`$0` is `run`). The exit code of the command is propagated as the exit code of `run`, so shell chaining like `run test && run build` works as expected.

### Shell

`shell:` selects the shell that executes `run:` strings — at the top level for the whole file, or per command to override. Like `env:`, a command's `shell:` applies to its subcommands too, with the innermost declaration winning:

```yaml
shell: bash              # every command runs with `bash -c`
commands:
  pick:
    run: 'a=(x y z); echo "${a[1]}"'   # bash arrays work
  plain:
    shell: sh            # this command (and its subcommands) uses sh
    run: echo ok
```

- The value is a shell name or path (`bash`, `zsh`, `/opt/homebrew/bin/zsh`), invoked as `<shell> -c`; unset means `sh`. It is an executable, not a command line — extra options like `bash -x` are not supported.
- Dynamic values (`{run: ...}`) are evaluated with the same resolved shell as the command they belong to.

## License

MIT
