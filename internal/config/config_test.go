package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvSkipsExistingEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("FOO=from-file\nBAR=from-file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("FOO", "from-env")
	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv() error = %v", err)
	}
	if got := os.Getenv("FOO"); got != "from-env" {
		t.Fatalf("FOO = %q, want from-env", got)
	}
	if got := os.Getenv("BAR"); got != "from-file" {
		t.Fatalf("BAR = %q, want from-file", got)
	}
}

func TestLoadMasterKeyFromFile(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	path := filepath.Join(t.TempDir(), "master_key")
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("COMPOSEPILOT_MASTER_KEY", "")
	t.Setenv("COMPOSEPILOT_MASTER_KEY_FILE", path)
	got, err := loadMasterKey()
	if err != nil {
		t.Fatalf("loadMasterKey() error = %v", err)
	}
	if string(got) != string(key) {
		t.Fatalf("loadMasterKey() mismatch")
	}
}
