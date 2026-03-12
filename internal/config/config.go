package config

import (
	"bufio"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ListenAddr string
	DataDir    string
	DBPath     string
	Workspace  string
	MasterKey  []byte
}

func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, err
	}

	listen := flag.String("listen", envOrDefault("COMPOSEPILOT_LISTEN", ":8080"), "listen address")
	dataDir := flag.String("data-dir", envOrDefault("COMPOSEPILOT_DATA_DIR", "./data"), "data directory")
	workspace := flag.String("workspace", envOrDefault("COMPOSEPILOT_WORKSPACE", "./workspace"), "repository workspace")
	flag.Parse()

	key, err := loadMasterKey()
	if err != nil {
		return Config{}, err
	}

	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		return Config{}, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(*workspace, 0o755); err != nil {
		return Config{}, fmt.Errorf("create workspace dir: %w", err)
	}

	return Config{
		ListenAddr: *listen,
		DataDir:    *dataDir,
		DBPath:     filepath.Join(*dataDir, "composepilot.db"),
		Workspace:  *workspace,
		MasterKey:  key,
	}, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadMasterKey() ([]byte, error) {
	if path := strings.TrimSpace(os.Getenv("COMPOSEPILOT_MASTER_KEY_FILE")); path != "" {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read master key file: %w", err)
		}
		return decodeMasterKey(strings.TrimSpace(string(contents)))
	}

	keyB64 := strings.TrimSpace(os.Getenv("COMPOSEPILOT_MASTER_KEY"))
	if keyB64 == "" {
		return nil, errors.New("set COMPOSEPILOT_MASTER_KEY or COMPOSEPILOT_MASTER_KEY_FILE to a base64-encoded 32-byte key")
	}
	return decodeMasterKey(keyB64)
}

func decodeMasterKey(keyB64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("decode master key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("master key must decode to 32 bytes")
	}
	return key, nil
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid .env line: %q", line)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("invalid .env line: %q", line)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}
