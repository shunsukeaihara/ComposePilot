package httphandler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"composepilot/internal/config"
	"composepilot/internal/models"
)

func TestParseManagedFiles(t *testing.T) {
	files := parseManagedFiles(
		[]string{"config/app.conf", "", "nested/test.env"},
		[]string{"one=1\n", "ignored", "two=2\n"},
	)
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if files[0].Path != "config/app.conf" || files[0].Content != "one=1\n" {
		t.Fatalf("files[0] = %#v", files[0])
	}
	if files[1].Path != "nested/test.env" || files[1].Content != "two=2\n" {
		t.Fatalf("files[1] = %#v", files[1])
	}
}

func TestMaterializeManagedFiles(t *testing.T) {
	workDir := t.TempDir()
	s := &Server{}
	svc := models.Service{
		WorkDir: workDir,
		ManagedFiles: []models.ManagedFile{
			{Path: "config/app.conf", Content: "listen=8080\n"},
			{Path: "env/prod/app.env", Content: "APP_ENV=prod\n"},
		},
	}

	output, err := s.materializeManagedFiles(context.Background(), svc, nil)
	if err != nil {
		t.Fatalf("materializeManagedFiles() error = %v", err)
	}
	if output == "" {
		t.Fatal("materializeManagedFiles() output is empty")
	}

	content, err := os.ReadFile(filepath.Join(workDir, "config/app.conf"))
	if err != nil {
		t.Fatalf("ReadFile(config/app.conf) error = %v", err)
	}
	if string(content) != "listen=8080\n" {
		t.Fatalf("config/app.conf = %q", string(content))
	}

	content, err = os.ReadFile(filepath.Join(workDir, "env/prod/app.env"))
	if err != nil {
		t.Fatalf("ReadFile(env/prod/app.env) error = %v", err)
	}
	if string(content) != "APP_ENV=prod\n" {
		t.Fatalf("env/prod/app.env = %q", string(content))
	}
}

func TestValidateManagedFilesRejectsTraversal(t *testing.T) {
	err := validateManagedFiles([]models.ManagedFile{{Path: "../escape.conf", Content: "x=1\n"}})
	if err == nil {
		t.Fatal("validateManagedFiles() error = nil, want error")
	}
}

func TestDisplayWorkDir(t *testing.T) {
	got := displayWorkDir(filepath.Join("sample_project"))
	if got != "sample_project" {
		t.Fatalf("displayWorkDir() = %q, want %q", got, "sample_project")
	}
}

func TestResolveWorkDirAvoidsDoubleWorkspacePrefix(t *testing.T) {
	s := &Server{cfg: config.Config{Workspace: "./workspace"}}
	got := s.resolveWorkDir(serviceRequest{WorkDir: "workspace/sample_project"})
	want := filepath.Clean("sample_project")
	if got != want {
		t.Fatalf("resolveWorkDir() = %q, want %q", got, want)
	}
}
