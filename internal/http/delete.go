package httphandler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) deleteProject(ctx context.Context, id int64, keepWorkspace bool) error {
	svc, err := s.store.GetService(ctx, id)
	if err != nil {
		return err
	}
	svc.WorkDir = s.normalizeStoredWorkDir(svc.WorkDir)
	workDir := filepath.Clean(s.serviceWorkDir(svc.WorkDir))
	workspaceRoot := filepath.Clean(s.serviceWorkDir(""))
	rel, err := filepath.Rel(workspaceRoot, workDir)
	if err != nil {
		return fmt.Errorf("resolve workspace path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to delete workdir outside workspace: %s", workDir)
	}
	if _, err := s.docker.Compose(ctx, workDir, svc.ComposeFiles, svc.Environment, "down"); err != nil {
		return fmt.Errorf("docker compose down: %w", err)
	}
	if !keepWorkspace && rel != "." {
		if err := os.RemoveAll(workDir); err != nil {
			return fmt.Errorf("remove workspace %s: %w", workDir, err)
		}
	}
	return s.store.DeleteService(ctx, id)
}
