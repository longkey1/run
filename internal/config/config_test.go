package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

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
		want    map[string]Task
		wantErr bool
	}{
		{
			name: "valid",
			content: `
tasks:
  build:
    description: Build the project
    command: go build -o bin/app
  test:
    command: go test ./...
`,
			want: map[string]Task{
				"build": {Description: "Build the project", Command: "go build -o bin/app"},
				"test":  {Command: "go test ./..."},
			},
		},
		{
			name: "nested tasks",
			content: `
tasks:
  deploy:
    description: Deploy the app
    tasks:
      staging:
        command: ./deploy.sh staging
      production:
        description: Deploy to production
        command: ./deploy.sh production
`,
			want: map[string]Task{
				"deploy": {
					Description: "Deploy the app",
					Tasks: map[string]Task{
						"staging":    {Command: "./deploy.sh staging"},
						"production": {Description: "Deploy to production", Command: "./deploy.sh production"},
					},
				},
			},
		},
		{
			name: "nested task with command and subtasks",
			content: `
tasks:
  db:
    command: ./db.sh status
    tasks:
      migrate:
        command: ./db.sh migrate
`,
			want: map[string]Task{
				"db": {
					Command: "./db.sh status",
					Tasks: map[string]Task{
						"migrate": {Command: "./db.sh migrate"},
					},
				},
			},
		},
		{
			name: "task with args",
			content: `
tasks:
  deploy:
    command: ./deploy.sh "$env" "$region"
    args:
      - name: env
        description: target environment
      - name: region
        default: us-east-1
      - name: empty-default
        default: ""
`,
			want: map[string]Task{
				"deploy": {
					Command: `./deploy.sh "$env" "$region"`,
					Args: []Arg{
						{Name: "env", Description: "target environment"},
						{Name: "region", Default: ptr("us-east-1")},
						{Name: "empty-default", Default: ptr("")},
					},
				},
			},
		},
		{
			name:    "args without command",
			content: "tasks:\n  deploy:\n    args:\n      - name: env\n    tasks:\n      staging:\n        command: ./deploy.sh staging\n",
			wantErr: true,
		},
		{
			name:    "arg without name",
			content: "tasks:\n  deploy:\n    command: ./deploy.sh\n    args:\n      - default: prod\n",
			wantErr: true,
		},
		{
			name:    "duplicate arg names",
			content: "tasks:\n  deploy:\n    command: ./deploy.sh\n    args:\n      - name: env\n      - name: env\n",
			wantErr: true,
		},
		{
			name:    "required arg after default",
			content: "tasks:\n  deploy:\n    command: ./deploy.sh\n    args:\n      - name: env\n        default: prod\n      - name: region\n",
			wantErr: true,
		},
		{
			name:    "no tasks",
			content: "tasks: {}\n",
			wantErr: true,
		},
		{
			name:    "missing command",
			content: "tasks:\n  build:\n    description: no command\n",
			wantErr: true,
		},
		{
			name:    "nested task missing command and subtasks",
			content: "tasks:\n  deploy:\n    tasks:\n      staging:\n        description: no command\n",
			wantErr: true,
		},
		{
			name:    "broken yaml",
			content: "tasks: [\n",
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
			if !reflect.DeepEqual(got.Tasks, tt.want) {
				t.Errorf("Load() tasks = %+v, want %+v", got.Tasks, tt.want)
			}
		})
	}
}

func TestLoadExternalFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		entry   string
		want    map[string]Task
		wantErr string
	}{
		{
			name: "basic expansion",
			files: map[string]string{
				".run.yaml": `
tasks:
  deploy:
    description: Deploy tasks
    file: ./deploy.run.yaml
`,
				"deploy.run.yaml": `
tasks:
  staging:
    command: ./deploy.sh staging
  production:
    description: Deploy to production
    command: ./deploy.sh production
`,
			},
			entry: ".run.yaml",
			want: map[string]Task{
				"deploy": {
					Description: "Deploy tasks",
					Tasks: map[string]Task{
						"staging":    {Command: "./deploy.sh staging"},
						"production": {Description: "Deploy to production", Command: "./deploy.sh production"},
					},
				},
			},
		},
		{
			name: "file with command",
			files: map[string]string{
				".run.yaml": `
tasks:
  db:
    command: ./db.sh status
    file: ./db.run.yaml
`,
				"db.run.yaml": `
tasks:
  migrate:
    command: ./db.sh migrate
`,
			},
			entry: ".run.yaml",
			want: map[string]Task{
				"db": {
					Command: "./db.sh status",
					Tasks: map[string]Task{
						"migrate": {Command: "./db.sh migrate"},
					},
				},
			},
		},
		{
			name: "nested external file resolves relative to its own dir",
			files: map[string]string{
				".run.yaml": `
tasks:
  deploy:
    file: ./sub/deploy.run.yaml
`,
				"sub/deploy.run.yaml": `
tasks:
  app:
    file: ./more.run.yaml
`,
				"sub/more.run.yaml": `
tasks:
  staging:
    command: ./deploy.sh staging
`,
			},
			entry: ".run.yaml",
			want: map[string]Task{
				"deploy": {
					Tasks: map[string]Task{
						"app": {
							Tasks: map[string]Task{
								"staging": {Command: "./deploy.sh staging"},
							},
						},
					},
				},
			},
		},
		{
			name: "file on nested inline task",
			files: map[string]string{
				".run.yaml": `
tasks:
  ops:
    tasks:
      deploy:
        file: ./deploy.run.yaml
`,
				"deploy.run.yaml": `
tasks:
  staging:
    command: ./deploy.sh staging
`,
			},
			entry: ".run.yaml",
			want: map[string]Task{
				"ops": {
					Tasks: map[string]Task{
						"deploy": {
							Tasks: map[string]Task{
								"staging": {Command: "./deploy.sh staging"},
							},
						},
					},
				},
			},
		},
		{
			name: "file and inline tasks are mutually exclusive",
			files: map[string]string{
				".run.yaml": `
tasks:
  deploy:
    file: ./deploy.run.yaml
    tasks:
      staging:
        command: ./deploy.sh staging
`,
			},
			entry:   ".run.yaml",
			wantErr: "file and tasks are mutually exclusive",
		},
		{
			name: "missing external file",
			files: map[string]string{
				".run.yaml": `
tasks:
  deploy:
    file: ./nosuch.yaml
`,
			},
			entry:   ".run.yaml",
			wantErr: `task "deploy"`,
		},
		{
			name: "broken yaml in external file",
			files: map[string]string{
				".run.yaml": `
tasks:
  deploy:
    file: ./deploy.run.yaml
`,
				"deploy.run.yaml": "tasks: [\n",
			},
			entry:   ".run.yaml",
			wantErr: "failed to parse",
		},
		{
			name: "empty external tasks",
			files: map[string]string{
				".run.yaml": `
tasks:
  deploy:
    file: ./deploy.run.yaml
`,
				"deploy.run.yaml": "tasks: {}\n",
			},
			entry:   ".run.yaml",
			wantErr: "no tasks defined in",
		},
		{
			name: "invalid task inside external file",
			files: map[string]string{
				".run.yaml": `
tasks:
  deploy:
    file: ./deploy.run.yaml
`,
				"deploy.run.yaml": `
tasks:
  staging:
    description: no command
`,
			},
			entry:   ".run.yaml",
			wantErr: `task "deploy staging" has no command or subtasks`,
		},
		{
			name: "circular reference between files",
			files: map[string]string{
				".run.yaml": `
tasks:
  a:
    file: ./b.run.yaml
`,
				"b.run.yaml": `
tasks:
  b:
    file: ./.run.yaml
`,
			},
			entry:   ".run.yaml",
			wantErr: "circular task file reference",
		},
		{
			name: "self reference via non-clean path",
			files: map[string]string{
				".run.yaml": `
tasks:
  a:
    file: ./sub/../.run.yaml
`,
			},
			entry:   ".run.yaml",
			wantErr: "circular task file reference",
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
			if !reflect.DeepEqual(got.Tasks, tt.want) {
				t.Errorf("Load() tasks = %+v, want %+v", got.Tasks, tt.want)
			}
		})
	}
}

func TestLoadExternalFileAbsolutePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	external := filepath.Join(dir, "deploy.run.yaml")
	writeFile(t, external, "tasks:\n  staging:\n    command: ./deploy.sh staging\n")
	entry := filepath.Join(dir, ".run.yaml")
	writeFile(t, entry, "tasks:\n  deploy:\n    file: "+external+"\n")

	got, err := Load(entry)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := map[string]Task{
		"deploy": {
			Tasks: map[string]Task{
				"staging": {Command: "./deploy.sh staging"},
			},
		},
	}
	if !reflect.DeepEqual(got.Tasks, want) {
		t.Errorf("Load() tasks = %+v, want %+v", got.Tasks, want)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	t.Parallel()

	if _, err := Load(filepath.Join(t.TempDir(), "nosuch.yaml")); err == nil {
		t.Error("Load() error = nil, want error")
	}
}
