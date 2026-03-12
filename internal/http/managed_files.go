package httphandler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"composepilot/internal/models"
)

func validateManagedFiles(files []models.ManagedFile) error {
	for i, file := range files {
		if strings.TrimSpace(file.Path) == "" {
			return fmt.Errorf("managedFiles[%d].path is required", i)
		}
		if _, err := cleanManagedFilePath(file.Path); err != nil {
			return fmt.Errorf("managedFiles[%d].path: %w", i, err)
		}
	}
	return nil
}

func cleanManagedFilePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.Contains(path, "\\") {
		path = strings.ReplaceAll(path, "\\", "/")
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return "", fmt.Errorf("path must point to a file")
	}
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must stay within the project directory")
	}
	return clean, nil
}

func (s *Server) materializeManagedFiles(ctx context.Context, svc models.Service, stream func(string)) (string, error) {
	var output strings.Builder
	for _, file := range svc.ManagedFiles {
		select {
		case <-ctx.Done():
			return output.String(), ctx.Err()
		default:
		}
		relPath, err := cleanManagedFilePath(file.Path)
		if err != nil {
			return output.String(), err
		}
		targetPath := filepath.Join(s.serviceWorkDir(svc.WorkDir), relPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return output.String(), fmt.Errorf("create managed file dir %q: %w", relPath, err)
		}
		if err := os.WriteFile(targetPath, []byte(file.Content), 0o644); err != nil {
			return output.String(), fmt.Errorf("write managed file %q: %w", relPath, err)
		}
		line := "$ write " + relPath + "\n"
		output.WriteString(line)
		if stream != nil {
			stream(line)
		}
	}
	return output.String(), nil
}
