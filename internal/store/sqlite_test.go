package store

import (
	"context"
	"path/filepath"
	"testing"

	"composepilot/internal/models"
)

func TestServiceRoundTrip(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "composepilot.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer st.Close()

	svc, err := st.CreateService(context.Background(), models.Service{
		Name:         "demo",
		RepoURL:      "git@github.com:org/repo.git",
		Branch:       "main",
		WorkDir:      "/tmp/demo",
		ComposeFiles: []string{"docker-compose.yml", "docker-compose.prod.yml"},
		Environment:  map[string]string{"APP_ENV": "prod"},
		ManagedFiles: []models.ManagedFile{
			{Path: "config/app.conf", Content: "name=demo\n"},
		},
		EncryptedSSHKey: "ciphertext",
	})
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}

	got, err := st.GetService(context.Background(), svc.ID)
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}
	if got.Name != svc.Name || got.RepoURL != svc.RepoURL || got.Branch != svc.Branch {
		t.Fatalf("GetService() mismatch = %#v", got)
	}
	if len(got.ComposeFiles) != 2 || got.ComposeFiles[1] != "docker-compose.prod.yml" {
		t.Fatalf("ComposeFiles = %#v", got.ComposeFiles)
	}
	if got.Environment["APP_ENV"] != "prod" {
		t.Fatalf("Environment = %#v", got.Environment)
	}
	if len(got.ManagedFiles) != 1 || got.ManagedFiles[0].Path != "config/app.conf" || got.ManagedFiles[0].Content != "name=demo\n" {
		t.Fatalf("ManagedFiles = %#v", got.ManagedFiles)
	}
}
