package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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

func TestLoadFileNotFound(t *testing.T) {
	t.Parallel()

	if _, err := Load(filepath.Join(t.TempDir(), "nosuch.yaml")); err == nil {
		t.Error("Load() error = nil, want error")
	}
}
