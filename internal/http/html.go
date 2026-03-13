package httphandler

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"composepilot/internal/models"
)

//go:embed all:templates
var templateFS embed.FS

var pageTemplates = template.Must(template.New("pages").Funcs(template.FuncMap{
	"joinLines":             func(values []string) string { return strings.Join(values, "\n") },
	"joinComma":             func(values []string) string { return strings.Join(values, ", ") },
	"envText":               envText,
	"envPairs":              envPairs,
	"composeFilesOrDefault": composeFilesOrDefault,
	"managedFilesList":      managedFilesList,
	"displayWorkDir":        displayWorkDir,
	"badgeClass": func(state, status string) string {
		value := strings.ToLower(strings.TrimSpace(state + " " + status))
		switch {
		case strings.Contains(value, "running"):
			return ""
		case value == "":
			return "unknown"
		default:
			return "stopped"
		}
	},
}).ParseFS(templateFS, "templates/*.gohtml"))

type viewData struct {
	Title           string
	ContentTemplate string
	Workspace       string
	Services        []models.Service
	Networks        []models.DockerNetwork
	Project         *projectViewData
	Flash           string
}

type projectViewData struct {
	Service           models.Service
	Containers        []models.Container
	SelectedContainer string
	Tab               string
	DeployOutput      string
}

type envPair struct {
	Key   string
	Value string
}

func envText(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	lines := make([]string, 0, len(env))
	for key, value := range env {
		lines = append(lines, key+"="+value)
	}
	return strings.Join(lines, "\n")
}

func envPairs(env map[string]string) []envPair {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	pairs := make([]envPair, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, envPair{Key: key, Value: env[key]})
	}
	return pairs
}

func composeFilesOrDefault(files []string) []string {
	if len(files) > 0 {
		return files
	}
	return []string{"docker-compose.yml"}
}

func managedFilesList(files []models.ManagedFile) []models.ManagedFile {
	if len(files) == 0 {
		return nil
	}
	return files
}

func displayWorkDir(workDir string) string {
	workDir = filepath.Clean(strings.TrimSpace(workDir))
	if workDir == "." {
		return ""
	}
	return workDir
}

func (s *Server) handleHomePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data, err := s.newViewData(r, "content-home")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.renderTemplate(w, r, data)
}

func (s *Server) handleNewProjectPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data, err := s.newViewData(r, "content-project-new")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.renderTemplate(w, r, data)
}

func (s *Server) handleCreateProjectPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	req, err := s.serviceRequestFromForm(r)
	if err != nil {
		s.renderFormError(w, r, "content-project-new", err, nil)
		return
	}
	svc, err := s.createService(r.Context(), req)
	if err != nil {
		s.renderFormError(w, r, "content-project-new", err, nil)
		return
	}
	s.redirectAfterHTMX(w, r, "/projects/"+strconv.FormatInt(svc.ID, 10))
}

func (s *Server) handleProjectPages(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/projects/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.renderProjectPage(w, r, id, r.URL.Query().Get("tab"), "")
		return
	}
	switch parts[1] {
	case "settings":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.FormValue("submitAction") == "delete" || r.FormValue("submitAction") == "delete-keep-workspace" {
			keepWorkspace := r.FormValue("submitAction") == "delete-keep-workspace"
			if err := s.deleteProject(r.Context(), id, keepWorkspace); err != nil {
				s.renderProjectFormError(w, r, id, err)
				return
			}
			s.redirectAfterHTMX(w, r, "/")
			return
		}
		req, err := s.serviceRequestFromForm(r)
		if err != nil {
			s.renderProjectFormError(w, r, id, err)
			return
		}
		updated, err := s.updateService(r.Context(), id, req)
		if err != nil {
			s.renderProjectFormError(w, r, id, err)
			return
		}
		if r.FormValue("submitAction") == "save-deploy" {
			plainKey, err := s.cipher.DecryptString(updated.EncryptedSSHKey)
			if err != nil {
				s.renderProjectFormError(w, r, id, err)
				return
			}
			output, err := s.deploy(r.Context(), updated, plainKey)
			if err != nil && output == "" {
				output = err.Error()
			}
			s.renderProjectPage(w, r, id, "deploy", output)
			return
		}
		s.redirectAfterHTMX(w, r, "/projects/"+strconv.FormatInt(id, 10)+"?tab=settings")
	case "actions":
		if len(parts) < 3 || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		action := parts[2]
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
		output := job.Output
		if err != nil && output == "" {
			output = err.Error()
		}
		s.renderProjectPage(w, r, id, "deploy", output)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleNetworksPage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := s.newViewData(r, "content-networks")
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}
		networks, err := s.docker.ListNetworks(r.Context())
		if err != nil {
			s.writeError(w, http.StatusBadGateway, err)
			return
		}
		data.Networks = networks
		s.renderTemplate(w, r, data)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			data, _ := s.newViewData(r, "content-networks")
			data.Flash = "network name is required"
			s.renderTemplate(w, r, data)
			return
		}
		_, err := s.docker.CreateNetwork(r.Context(), name)
		if err != nil {
			s.writeError(w, http.StatusBadGateway, err)
			return
		}
		data, err := s.newViewData(r, "content-networks")
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}
		data.Flash = "Network created."
		data.Networks, _ = s.docker.ListNetworks(r.Context())
		s.renderTemplate(w, r, data)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) newViewData(r *http.Request, content string) (viewData, error) {
	services, err := s.store.ListServices(r.Context())
	if err != nil {
		return viewData{}, err
	}
	for i := range services {
		services[i].WorkDir = s.normalizeStoredWorkDir(services[i].WorkDir)
	}
	return viewData{Title: "ComposePilot", ContentTemplate: content, Workspace: s.cfg.Workspace, Services: services}, nil
}

func (s *Server) renderProjectPage(w http.ResponseWriter, r *http.Request, id int64, tab string, deployOutput string) {
	if tab == "" {
		tab = "settings"
	}
	data, err := s.newViewData(r, "content-project-detail")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	svc, err := s.store.GetService(r.Context(), id)
	if err != nil {
		s.writeError(w, statusForError(err), err)
		return
	}
	svc.WorkDir = s.normalizeStoredWorkDir(svc.WorkDir)
	containers, _ := s.docker.ListContainers(r.Context(), s.serviceWorkDir(svc.WorkDir), svc.ComposeFiles, svc.Environment)
	selectedContainer := r.URL.Query().Get("container")
	data.Project = &projectViewData{Service: svc, Containers: containers, SelectedContainer: selectedContainer, Tab: tab, DeployOutput: deployOutput}
	s.renderTemplate(w, r, data)
}

func (s *Server) renderProjectFormError(w http.ResponseWriter, r *http.Request, id int64, formErr error) {
	data, err := s.newViewData(r, "content-project-detail")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	svc, getErr := s.store.GetService(r.Context(), id)
	if getErr != nil {
		s.writeError(w, statusForError(getErr), getErr)
		return
	}
	svc.WorkDir = s.normalizeStoredWorkDir(svc.WorkDir)
	containers, _ := s.docker.ListContainers(r.Context(), s.serviceWorkDir(svc.WorkDir), svc.ComposeFiles, svc.Environment)
	data.Project = &projectViewData{Service: svc, Containers: containers, Tab: "settings"}
	data.Flash = formErr.Error()
	s.renderTemplate(w, r, data)
}

func (s *Server) renderFormError(w http.ResponseWriter, r *http.Request, content string, formErr error, project *projectViewData) {
	data, err := s.newViewData(r, content)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	data.Project = project
	data.Flash = formErr.Error()
	s.renderTemplate(w, r, data)
}

func (s *Server) renderTemplate(w http.ResponseWriter, r *http.Request, data viewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	name := data.ContentTemplate
	if !isHTMX(r) {
		name = "layout"
	}
	if err := pageTemplates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) serviceRequestFromForm(r *http.Request) (serviceRequest, error) {
	if err := r.ParseForm(); err != nil {
		return serviceRequest{}, err
	}
	return serviceRequest{
		Name:         strings.TrimSpace(r.FormValue("name")),
		RepoURL:      strings.TrimSpace(r.FormValue("repoUrl")),
		Branch:       strings.TrimSpace(r.FormValue("branch")),
		WorkDir:      strings.TrimSpace(r.FormValue("workDir")),
		ComposeFiles: parseComposeFiles(r.Form["composeFile"]),
		Environment:  parseEnvRows(r.Form["envKey"], r.Form["envValue"]),
		ManagedFiles: parseManagedFiles(r.Form["managedFilePath"], r.Form["managedFileContent"]),
		DeployKey:    r.FormValue("deployKey"),
	}, nil
}

func parseComposeFiles(values []string) []string {
	files := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		files = append(files, value)
	}
	return files
}

func splitNonEmptyLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseEnvLines(value string) map[string]string {
	env := map[string]string{}
	for _, line := range splitNonEmptyLines(value) {
		key, val, ok := strings.Cut(line, "=")
		if ok {
			env[strings.TrimSpace(key)] = strings.TrimSpace(val)
		}
	}
	return env
}

func parseEnvRows(keys, values []string) map[string]string {
	env := map[string]string{}
	maxLen := len(keys)
	if len(values) > maxLen {
		maxLen = len(values)
	}
	for i := 0; i < maxLen; i++ {
		var key, value string
		if i < len(keys) {
			key = strings.TrimSpace(keys[i])
		}
		if i < len(values) {
			value = strings.TrimSpace(values[i])
		}
		if key == "" {
			continue
		}
		env[key] = value
	}
	return env
}

func parseManagedFiles(paths, contents []string) []models.ManagedFile {
	maxLen := len(paths)
	if len(contents) > maxLen {
		maxLen = len(contents)
	}
	files := make([]models.ManagedFile, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		var path, content string
		if i < len(paths) {
			path = strings.TrimSpace(paths[i])
		}
		if i < len(contents) {
			content = contents[i]
		}
		if path == "" {
			continue
		}
		files = append(files, models.ManagedFile{
			Path:    path,
			Content: content,
		})
	}
	return files
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func (s *Server) redirectAfterHTMX(w http.ResponseWriter, r *http.Request, location string) {
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", location)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}
