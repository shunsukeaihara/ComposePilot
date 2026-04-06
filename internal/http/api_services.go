package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"composepilot/internal/models"
)

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		services, err := s.store.ListServices(r.Context())
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}
		s.writeJSON(w, http.StatusOK, services)
	case http.MethodPost:
		var req serviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		svc, err := s.createService(r.Context(), req)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		s.writeJSON(w, http.StatusCreated, svc)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleServiceByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/services/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(parts) == 1 {
		s.handleServiceResource(w, r, id)
		return
	}
	if parts[1] == "containers" {
		if len(parts) == 2 {
			s.handleContainers(w, r, id)
			return
		}
		if len(parts) == 4 && parts[3] == "restart" {
			s.handleContainerRestart(w, r, id, parts[2])
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if parts[1] == "actions" {
		if len(parts) == 4 && parts[3] == "stream" {
			s.handleServiceActionStream(w, r, id, parts[2])
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	action := parts[1]
	switch action {
	case "deploy", "pull", "build", "restart", "stop", "down":
		s.handleServiceAction(w, r, id, action)
	case "logs":
		s.handleLogs(w, r, id)
	case "exec":
		s.handleExec(w, r, id)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *Server) handleServiceResource(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case http.MethodGet:
		svc, err := s.store.GetService(r.Context(), id)
		if err != nil {
			s.writeError(w, statusForError(err), err)
			return
		}
		s.writeJSON(w, http.StatusOK, svc)
	case http.MethodPut:
		var req serviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		svc, err := s.updateService(r.Context(), id, req)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		s.writeJSON(w, http.StatusOK, svc)
	case http.MethodDelete:
		keepWorkspace := r.URL.Query().Get("keepWorkspace") == "true"
		if err := s.deleteProject(r.Context(), id, keepWorkspace); err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) createService(ctx context.Context, req serviceRequest) (models.Service, error) {
	if err := validateServiceRequest(req); err != nil {
		return models.Service{}, err
	}
	if req.Environment == nil {
		req.Environment = map[string]string{}
	}
	if req.ManagedFiles == nil {
		req.ManagedFiles = []models.ManagedFile{}
	}
	encKey, err := s.cipher.EncryptString(req.DeployKey)
	if err != nil {
		return models.Service{}, fmt.Errorf("encrypt deploy key: %w", err)
	}
	service := models.Service{
		Name:            req.Name,
		RepoURL:         req.RepoURL,
		Branch:          req.Branch,
		WorkDir:         s.resolveWorkDir(req),
		ComposeFiles:    req.ComposeFiles,
		Environment:     req.Environment,
		ManagedFiles:    req.ManagedFiles,
		EncryptedSSHKey: encKey,
	}
	created, err := s.store.CreateService(ctx, service)
	if err != nil {
		return models.Service{}, err
	}
	plainKey, err := s.cipher.DecryptString(created.EncryptedSSHKey)
	if err != nil {
		return models.Service{}, err
	}
	workDir := s.serviceWorkDir(created.WorkDir)
	_, err = s.jobs.Run(ctx, created.ID, "clone", func(ctx context.Context) (string, error) {
		output, err := s.git.EnsureCloned(ctx, created.RepoURL, created.Branch, workDir, plainKey)
		if err != nil {
			return output, err
		}
		chunk, err := s.materializeManagedFiles(ctx, created, nil)
		return output + chunk, err
	})
	if err != nil {
		_ = s.store.DeleteService(ctx, created.ID)
		return created, err
	}
	return created, nil
}

func (s *Server) updateService(ctx context.Context, id int64, req serviceRequest) (models.Service, error) {
	if err := validateServiceRequest(req); err != nil {
		return models.Service{}, err
	}
	existing, err := s.store.GetService(ctx, id)
	if err != nil {
		return models.Service{}, err
	}
	if req.Environment == nil {
		req.Environment = map[string]string{}
	}
	if req.ManagedFiles == nil {
		req.ManagedFiles = []models.ManagedFile{}
	}
	encKey := existing.EncryptedSSHKey
	if strings.TrimSpace(req.DeployKey) != "" {
		encKey, err = s.cipher.EncryptString(req.DeployKey)
		if err != nil {
			return models.Service{}, err
		}
	}
	existing.Name = req.Name
	existing.RepoURL = req.RepoURL
	existing.Branch = req.Branch
	existing.WorkDir = s.resolveWorkDir(req)
	existing.ComposeFiles = req.ComposeFiles
	existing.Environment = req.Environment
	existing.ManagedFiles = req.ManagedFiles
	existing.EncryptedSSHKey = encKey
	return s.store.UpdateService(ctx, existing)
}

func validateServiceRequest(req serviceRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(req.RepoURL) == "" {
		return errors.New("repoUrl is required")
	}
	if strings.TrimSpace(req.Branch) == "" {
		return errors.New("branch is required")
	}
	if len(req.ComposeFiles) == 0 {
		return errors.New("at least one compose file is required")
	}
	for _, composeFile := range req.ComposeFiles {
		if strings.TrimSpace(composeFile) == "" {
			return errors.New("compose file entries must not be empty")
		}
	}
	if err := validateManagedFiles(req.ManagedFiles); err != nil {
		return err
	}
	return nil
}

func (s *Server) resolveWorkDir(req serviceRequest) string {
	if strings.TrimSpace(req.WorkDir) != "" {
		workDir := filepath.Clean(strings.TrimSpace(req.WorkDir))
		workspace := filepath.Clean(s.cfg.Workspace)
		if filepath.IsAbs(workDir) {
			if rel, err := filepath.Rel(workspace, workDir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				if rel == "." {
					return ""
				}
				return rel
			}
			return filepath.Base(workDir)
		}
		if workDir == "." || workDir == "" {
			return ""
		}
		if workDir == workspace || strings.HasPrefix(workDir, workspace+string(filepath.Separator)) {
			rel, err := filepath.Rel(workspace, workDir)
			if err == nil {
				if rel == "." {
					return ""
				}
				return rel
			}
		}
		return workDir
	}
	return slugify(req.Name)
}

func (s *Server) ResolveServiceWorkDir(workDir string) string {
	return s.serviceWorkDir(workDir)
}

func (s *Server) serviceWorkDir(workDir string) string {
	workDir = filepath.Clean(strings.TrimSpace(workDir))
	if workDir == "." || workDir == "" {
		return filepath.Clean(s.cfg.Workspace)
	}
	if filepath.IsAbs(workDir) {
		return workDir
	}
	return filepath.Join(filepath.Clean(s.cfg.Workspace), workDir)
}

func (s *Server) normalizeStoredWorkDir(workDir string) string {
	workDir = filepath.Clean(strings.TrimSpace(workDir))
	workspace := filepath.Clean(s.cfg.Workspace)
	if workDir == "." || workDir == "" {
		return ""
	}
	if filepath.IsAbs(workDir) {
		if rel, err := filepath.Rel(workspace, workDir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if rel == "." {
				return ""
			}
			return rel
		}
		return workDir
	}
	if workDir == workspace || strings.HasPrefix(workDir, workspace+string(filepath.Separator)) {
		if rel, err := filepath.Rel(workspace, workDir); err == nil {
			if rel == "." {
				return ""
			}
			return rel
		}
	}
	return workDir
}

func slugify(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, name)
}

func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request, id int64, action string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	svc, plainKey, err := s.loadService(r.Context(), id)
	if err != nil {
		s.writeError(w, statusForError(err), err)
		return
	}
	job, err := s.jobs.Run(r.Context(), id, action, func(ctx context.Context) (string, error) {
		workDir := s.serviceWorkDir(svc.WorkDir)
		switch action {
		case "deploy":
			return s.deploy(ctx, svc, plainKey)
		case "pull":
			output, err := s.git.CheckoutAndPull(ctx, workDir, svc.Branch, plainKey)
			if err != nil {
				return output, err
			}
			chunk, err := s.materializeManagedFiles(ctx, svc, nil)
			return output + chunk, err
		case "build":
			return s.docker.Compose(ctx, workDir, svc.ComposeFiles, svc.Environment, "build")
		case "restart":
			return s.docker.Compose(ctx, workDir, svc.ComposeFiles, svc.Environment, "restart")
		case "stop":
			return s.docker.Compose(ctx, workDir, svc.ComposeFiles, svc.Environment, "stop")
		case "down":
			return s.docker.Compose(ctx, workDir, svc.ComposeFiles, svc.Environment, "down")
		default:
			return "", fmt.Errorf("unsupported action %q", action)
		}
	})
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, job)
		return
	}
	s.writeJSON(w, http.StatusOK, job)
}

func (s *Server) deploy(ctx context.Context, svc models.Service, plainKey string) (string, error) {
	return s.deployStream(ctx, svc, plainKey, nil)
}

func (s *Server) deployStream(ctx context.Context, svc models.Service, plainKey string, stream func(string)) (string, error) {
	var output strings.Builder
	workDir := s.serviceWorkDir(svc.WorkDir)
	chunk, err := s.git.CheckoutAndPullStream(ctx, workDir, svc.Branch, plainKey, stream)
	output.WriteString(chunk)
	if err != nil {
		return output.String(), err
	}
	chunk, err = s.materializeManagedFiles(ctx, svc, stream)
	output.WriteString(chunk)
	if err != nil {
		return output.String(), err
	}
	chunk, err = s.docker.ComposeStream(ctx, workDir, svc.ComposeFiles, svc.Environment, stream, "build")
	output.WriteString(chunk)
	if err != nil {
		return output.String(), err
	}
	chunk, err = s.docker.ComposeStream(ctx, workDir, svc.ComposeFiles, svc.Environment, stream, "up", "-d")
	output.WriteString(chunk)
	return output.String(), err
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	svc, _, err := s.loadService(r.Context(), id)
	if err != nil {
		s.writeError(w, statusForError(err), err)
		return
	}
	containers, err := s.docker.ListContainers(r.Context(), s.serviceWorkDir(svc.WorkDir), svc.ComposeFiles, svc.Environment)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, err)
		return
	}
	s.writeJSON(w, http.StatusOK, containers)
}

func (s *Server) handleContainerRestart(w http.ResponseWriter, r *http.Request, id int64, name string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	svc, _, err := s.loadService(r.Context(), id)
	if err != nil {
		s.writeError(w, statusForError(err), err)
		return
	}
	containers, err := s.docker.ListContainers(r.Context(), s.serviceWorkDir(svc.WorkDir), svc.ComposeFiles, svc.Environment)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, err)
		return
	}
	allowed := false
	for _, container := range containers {
		if container.Name == name {
			allowed = true
			break
		}
	}
	if !allowed {
		s.writeError(w, http.StatusNotFound, fmt.Errorf("container %q not found", name))
		return
	}
	output, err := s.docker.RestartContainer(r.Context(), name)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, map[string]string{"output": output, "error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"output": output})
}

func (s *Server) loadService(ctx context.Context, id int64) (models.Service, string, error) {
	svc, err := s.store.GetService(ctx, id)
	if err != nil {
		return models.Service{}, "", err
	}
	svc.WorkDir = s.normalizeStoredWorkDir(svc.WorkDir)
	plainKey, err := s.cipher.DecryptString(svc.EncryptedSSHKey)
	if err != nil {
		return models.Service{}, "", err
	}
	return svc, plainKey, nil
}
