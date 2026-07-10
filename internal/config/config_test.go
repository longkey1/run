package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// lit and dyn build pointers to literal and dynamic values for
// expected defaults.
func lit(s string) *Value { return &Value{Literal: s} }
func dyn(s string) *Value { return &Value{Run: s} }

// writeFile creates a file with the given content, creating parent
// directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    map[string]Command
		wantEnv map[string]Value
		wantErr bool
	}{
		{
			name: "valid",
			content: `
commands:
  build:
    description: Build the project
    run: go build -o bin/app
  test:
    run: go test ./...
`,
			want: map[string]Command{
				"build": {Description: "Build the project", Run: "go build -o bin/app"},
				"test":  {Run: "go test ./..."},
			},
		},
		{
			name: "nested commands",
			content: `
commands:
  deploy:
    description: Deploy the app
    commands:
      staging:
        run: ./deploy.sh staging
      production:
        description: Deploy to production
        run: ./deploy.sh production
`,
			want: map[string]Command{
				"deploy": {
					Description: "Deploy the app",
					Commands: map[string]Command{
						"staging":    {Run: "./deploy.sh staging"},
						"production": {Description: "Deploy to production", Run: "./deploy.sh production"},
					},
				},
			},
		},
		{
			name: "nested command with run and subcommands",
			content: `
commands:
  db:
    run: ./db.sh status
    commands:
      migrate:
        run: ./db.sh migrate
`,
			want: map[string]Command{
				"db": {
					Run: "./db.sh status",
					Commands: map[string]Command{
						"migrate": {Run: "./db.sh migrate"},
					},
				},
			},
		},
		{
			name: "command with args",
			content: `
commands:
  deploy:
    run: ./deploy.sh "$env" "$region"
    args:
      - name: env
        description: target environment
      - name: region
        default: us-east-1
      - name: empty-default
        default: ""
`,
			want: map[string]Command{
				"deploy": {
					Run: `./deploy.sh "$env" "$region"`,
					Args: []Arg{
						{Name: "env", Description: "target environment"},
						{Name: "region", Default: lit("us-east-1")},
						{Name: "empty-default", Default: lit("")},
					},
				},
			},
		},
		{
			name: "command with flags",
			content: `
commands:
  deploy:
    run: ./deploy.sh
    flags:
      - name: force
        type: bool
        description: skip confirmation
      - name: from
        default: "2026-01-01"
      - name: mode
        type: string
      - name: empty
        default: ""
      - name: label
`,
			want: map[string]Command{
				"deploy": {
					Run: "./deploy.sh",
					Flags: []Flag{
						{Name: "force", Type: "bool", Description: "skip confirmation"},
						{Name: "from", Default: lit("2026-01-01")},
						{Name: "mode", Type: "string"},
						{Name: "empty", Default: lit("")},
						{Name: "label"},
					},
				},
			},
		},
		{
			name: "top-level and command env",
			content: `
env:
  GREETING: hello
  SCOPE: top
commands:
  deploy:
    env:
      SCOPE: command
    run: ./deploy.sh
    commands:
      staging:
        env:
          SCOPE: staging
          EMPTY: ""
        run: ./deploy.sh staging
`,
			want: map[string]Command{
				"deploy": {
					Env: map[string]Value{"SCOPE": {Literal: "command"}},
					Run: "./deploy.sh",
					Commands: map[string]Command{
						"staging": {
							Env: map[string]Value{"SCOPE": {Literal: "staging"}, "EMPTY": {}},
							Run: "./deploy.sh staging",
						},
					},
				},
			},
			wantEnv: map[string]Value{"GREETING": {Literal: "hello"}, "SCOPE": {Literal: "top"}},
		},
		{
			name: "dynamic env and defaults",
			content: `
env:
  TODAY:
    run: date +%F
commands:
  report:
    env:
      STAMP:
        run: date +%s
    args:
      - name: date
        default:
          run: echo "$TODAY"
    flags:
      - name: from
        default:
          run: echo start
    run: echo "$1"
`,
			want: map[string]Command{
				"report": {
					Env: map[string]Value{"STAMP": {Run: "date +%s"}},
					Args: []Arg{
						{Name: "date", Default: dyn(`echo "$TODAY"`)},
					},
					Flags: []Flag{
						{Name: "from", Default: dyn("echo start")},
					},
					Run: `echo "$1"`,
				},
			},
			wantEnv: map[string]Value{"TODAY": {Run: "date +%F"}},
		},
		{
			name:    "dynamic value with empty run",
			content: "commands:\n  build:\n    run: go build\n    env:\n      A:\n        run: \"\"\n",
			wantErr: true,
		},
		{
			name:    "dynamic value with unknown key",
			content: "commands:\n  build:\n    run: go build\n    env:\n      A:\n        run: date\n        extra: x\n",
			wantErr: true,
		},
		{
			name:    "sequence env value",
			content: "commands:\n  build:\n    run: go build\n    env:\n      A:\n        - x\n",
			wantErr: true,
		},
		{
			name:    "bool flag with dynamic default",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    flags:\n      - name: force\n        type: bool\n        default:\n          run: echo true\n",
			wantErr: true,
		},
		{
			name:    "top-level env with empty key",
			content: "env:\n  \"\": value\ncommands:\n  build:\n    run: go build\n",
			wantErr: true,
		},
		{
			name:    "command env with empty key",
			content: "commands:\n  build:\n    run: go build\n    env:\n      \"\": value\n",
			wantErr: true,
		},
		{
			name:    "env key containing equals",
			content: "commands:\n  build:\n    run: go build\n    env:\n      \"A=B\": value\n",
			wantErr: true,
		},
		{
			name:    "args without run",
			content: "commands:\n  deploy:\n    args:\n      - name: env\n    commands:\n      staging:\n        run: ./deploy.sh staging\n",
			wantErr: true,
		},
		{
			name:    "arg without name",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    args:\n      - default: prod\n",
			wantErr: true,
		},
		{
			name:    "duplicate arg names",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    args:\n      - name: env\n      - name: env\n",
			wantErr: true,
		},
		{
			name:    "required arg after default",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    args:\n      - name: env\n        default: prod\n      - name: region\n",
			wantErr: true,
		},
		{
			name:    "flags without run",
			content: "commands:\n  deploy:\n    flags:\n      - name: force\n        type: bool\n    commands:\n      staging:\n        run: ./deploy.sh staging\n",
			wantErr: true,
		},
		{
			name:    "flag without name",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    flags:\n      - type: bool\n",
			wantErr: true,
		},
		{
			name:    "duplicate flag names",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    flags:\n      - name: force\n      - name: force\n",
			wantErr: true,
		},
		{
			name:    "flag colliding with arg name",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    args:\n      - name: env\n    flags:\n      - name: env\n",
			wantErr: true,
		},
		{
			name:    "flag with invalid type",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    flags:\n      - name: count\n        type: int\n",
			wantErr: true,
		},
		{
			name:    "bool flag with default",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    flags:\n      - name: force\n        type: bool\n        default: \"true\"\n",
			wantErr: true,
		},
		{
			name:    "flag name containing equals",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    flags:\n      - name: \"a=b\"\n",
			wantErr: true,
		},
		{
			name:    "flag name with leading dash",
			content: "commands:\n  deploy:\n    run: ./deploy.sh\n    flags:\n      - name: \"-x\"\n",
			wantErr: true,
		},
		{
			name:    "no commands",
			content: "commands: {}\n",
			wantErr: true,
		},
		{
			name:    "reserved top-level command name self",
			content: "commands:\n  self:\n    run: echo self\n",
			wantErr: true,
		},
		{
			name:    "nested command may be named self",
			content: "commands:\n  deploy:\n    commands:\n      self:\n        run: ./deploy.sh self\n",
			want: map[string]Command{
				"deploy": {
					Commands: map[string]Command{
						"self": {Run: "./deploy.sh self"},
					},
				},
			},
		},
		{
			name:    "missing run",
			content: "commands:\n  build:\n    description: no run\n",
			wantErr: true,
		},
		{
			name:    "nested command missing run and subcommands",
			content: "commands:\n  deploy:\n    commands:\n      staging:\n        description: no run\n",
			wantErr: true,
		},
		{
			name:    "broken yaml",
			content: "commands: [\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), ".run.yaml")
			writeFile(t, path, tt.content)

			got, err := Load(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got.Commands, tt.want) {
				t.Errorf("Load() commands = %+v, want %+v", got.Commands, tt.want)
			}
			if tt.wantEnv != nil && !reflect.DeepEqual(got.Env, tt.wantEnv) {
				t.Errorf("Load() env = %+v, want %+v", got.Env, tt.wantEnv)
			}
		})
	}
}

func TestLoadIncludes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		entry   string
		want    map[string]Command
		wantErr string
	}{
		{
			name: "top-level include merges flat",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./common.yaml
commands:
  build:
    run: go build ./...
`,
				"common.yaml": `
commands:
  lint:
    run: golangci-lint run
  fmt:
    description: Format code
    run: gofmt -w .
`,
			},
			entry: ".run.yaml",
			want: map[string]Command{
				"build": {Run: "go build ./..."},
				"lint":  {Run: "golangci-lint run"},
				"fmt":   {Description: "Format code", Run: "gofmt -w ."},
			},
		},
		{
			name: "top-level include without local commands",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./common.yaml
`,
				"common.yaml": `
commands:
  lint:
    run: golangci-lint run
`,
			},
			entry: ".run.yaml",
			want: map[string]Command{
				"lint": {Run: "golangci-lint run"},
			},
		},
		{
			name: "command-level include nests as subcommands",
			files: map[string]string{
				".run.yaml": `
commands:
  deploy:
    description: Deploy commands
    includes:
      - ./deploy.yaml
`,
				"deploy.yaml": `
commands:
  staging:
    run: ./deploy.sh staging
  production:
    description: Deploy to production
    run: ./deploy.sh production
`,
			},
			entry: ".run.yaml",
			want: map[string]Command{
				"deploy": {
					Description: "Deploy commands",
					Commands: map[string]Command{
						"staging":    {Run: "./deploy.sh staging"},
						"production": {Description: "Deploy to production", Run: "./deploy.sh production"},
					},
				},
			},
		},
		{
			name: "include combined with run and local subcommands",
			files: map[string]string{
				".run.yaml": `
commands:
  db:
    run: ./db.sh status
    includes:
      - ./db.yaml
    commands:
      seed:
        run: ./db.sh seed
`,
				"db.yaml": `
commands:
  migrate:
    run: ./db.sh migrate
`,
			},
			entry: ".run.yaml",
			want: map[string]Command{
				"db": {
					Run: "./db.sh status",
					Commands: map[string]Command{
						"seed":    {Run: "./db.sh seed"},
						"migrate": {Run: "./db.sh migrate"},
					},
				},
			},
		},
		{
			name: "nested include resolves relative to its own dir",
			files: map[string]string{
				".run.yaml": `
commands:
  deploy:
    includes:
      - ./sub/deploy.yaml
`,
				"sub/deploy.yaml": `
commands:
  app:
    includes:
      - ./more.yaml
`,
				"sub/more.yaml": `
commands:
  staging:
    run: ./deploy.sh staging
`,
			},
			entry: ".run.yaml",
			want: map[string]Command{
				"deploy": {
					Commands: map[string]Command{
						"app": {
							Commands: map[string]Command{
								"staging": {Run: "./deploy.sh staging"},
							},
						},
					},
				},
			},
		},
		{
			name: "same file included from two branches",
			files: map[string]string{
				".run.yaml": `
commands:
  a:
    includes:
      - ./shared.yaml
  b:
    includes:
      - ./shared.yaml
`,
				"shared.yaml": `
commands:
  ping:
    run: echo pong
`,
			},
			entry: ".run.yaml",
			want: map[string]Command{
				"a": {Commands: map[string]Command{"ping": {Run: "echo pong"}}},
				"b": {Commands: map[string]Command{"ping": {Run: "echo pong"}}},
			},
		},
		{
			name: "included env applies to included commands only",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./inc.yaml
commands:
  local:
    run: echo local
`,
				"inc.yaml": `
env:
  A: file
  B: file
commands:
  inc:
    env:
      A: own
    run: echo inc
`,
			},
			entry: ".run.yaml",
			want: map[string]Command{
				"local": {Run: "echo local"},
				"inc": {
					Env: map[string]Value{"A": {Literal: "own"}, "B": {Literal: "file"}},
					Run: "echo inc",
				},
			},
		},
		{
			name: "inner include env wins over outer include env",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./outer.yaml
`,
				"outer.yaml": `
env:
  A: outer
  B: outer
includes:
  - ./inner.yaml
`,
				"inner.yaml": `
env:
  A: inner
commands:
  c:
    run: echo c
`,
			},
			entry: ".run.yaml",
			want: map[string]Command{
				"c": {
					Env: map[string]Value{"A": {Literal: "inner"}, "B": {Literal: "outer"}},
					Run: "echo c",
				},
			},
		},
		{
			name: "conflict with local command",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./inc.yaml
commands:
  build:
    run: go build ./...
`,
				"inc.yaml": `
commands:
  build:
    run: make build
`,
			},
			entry:   ".run.yaml",
			wantErr: `command "build" already defined`,
		},
		{
			name: "conflict between includes",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./a.yaml
  - ./b.yaml
`,
				"a.yaml": `
commands:
  lint:
    run: echo a
`,
				"b.yaml": `
commands:
  lint:
    run: echo b
`,
			},
			entry:   ".run.yaml",
			wantErr: `command "lint" already defined`,
		},
		{
			name: "missing included file",
			files: map[string]string{
				".run.yaml": `
commands:
  deploy:
    includes:
      - ./nosuch.yaml
`,
			},
			entry:   ".run.yaml",
			wantErr: `command "deploy"`,
		},
		{
			name: "broken yaml in included file",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./inc.yaml
`,
				"inc.yaml": "commands: [\n",
			},
			entry:   ".run.yaml",
			wantErr: "failed to parse",
		},
		{
			name: "included file without commands",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./inc.yaml
`,
				"inc.yaml": "env:\n  A: a\n",
			},
			entry:   ".run.yaml",
			wantErr: "no commands defined in",
		},
		{
			name: "invalid command inside included file",
			files: map[string]string{
				".run.yaml": `
commands:
  deploy:
    includes:
      - ./deploy.yaml
`,
				"deploy.yaml": `
commands:
  staging:
    description: no run
`,
			},
			entry:   ".run.yaml",
			wantErr: `command "deploy staging" has no run or subcommands`,
		},
		{
			name: "circular include between files",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./b.yaml
`,
				"b.yaml": `
commands:
  b:
    includes:
      - ./.run.yaml
`,
			},
			entry:   ".run.yaml",
			wantErr: "circular include",
		},
		{
			name: "self include via non-clean path",
			files: map[string]string{
				".run.yaml": `
includes:
  - ./sub/../.run.yaml
`,
			},
			entry:   ".run.yaml",
			wantErr: "circular include",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for path, content := range tt.files {
				writeFile(t, filepath.Join(dir, path), content)
			}

			got, err := Load(filepath.Join(dir, tt.entry))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Load() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(got.Commands, tt.want) {
				t.Errorf("Load() commands = %+v, want %+v", got.Commands, tt.want)
			}
		})
	}
}

func TestLoadIncludeAbsolutePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	external := filepath.Join(dir, "deploy.yaml")
	writeFile(t, external, "commands:\n  staging:\n    run: ./deploy.sh staging\n")
	entry := filepath.Join(dir, ".run.yaml")
	writeFile(t, entry, "commands:\n  deploy:\n    includes:\n      - "+external+"\n")

	got, err := Load(entry)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := map[string]Command{
		"deploy": {
			Commands: map[string]Command{
				"staging": {Run: "./deploy.sh staging"},
			},
		},
	}
	if !reflect.DeepEqual(got.Commands, want) {
		t.Errorf("Load() commands = %+v, want %+v", got.Commands, want)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	t.Parallel()

	if _, err := Load(filepath.Join(t.TempDir(), "nosuch.yaml")); err == nil {
		t.Error("Load() error = nil, want error")
	}
}
