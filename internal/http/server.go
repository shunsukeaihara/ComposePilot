package httphandler

import (
	"net/http"

	"github.com/gorilla/websocket"

	"composepilot/internal/config"
	cryptox "composepilot/internal/crypto"
	"composepilot/internal/dockerops"
	"composepilot/internal/gitops"
	"composepilot/internal/jobs"
	"composepilot/internal/models"
	"composepilot/internal/store"
)

type Server struct {
	cfg      config.Config
	store    *store.Store
	cipher   *cryptox.Cipher
	git      *gitops.Runner
	docker   *dockerops.Runner
	jobs     *jobs.Recorder
	mux      *http.ServeMux
	upgrader websocket.Upgrader
}

type serviceRequest struct {
	Name         string               `json:"name"`
	RepoURL      string               `json:"repoUrl"`
	Branch       string               `json:"branch"`
	WorkDir      string               `json:"workDir"`
	ComposeFiles []string             `json:"composeFiles"`
	Environment  map[string]string    `json:"environment"`
	ManagedFiles []models.ManagedFile `json:"managedFiles"`
	DeployKey    string               `json:"deployKey"`
}

type networkRequest struct {
	Name string `json:"name"`
}

func NewServer(cfg config.Config, st *store.Store, cipher *cryptox.Cipher) *Server {
	s := &Server{
		cfg:    cfg,
		store:  st,
		cipher: cipher,
		git:    gitops.NewRunner(),
		docker: dockerops.NewRunner(),
		jobs:   jobs.NewRecorder(),
		mux:    http.NewServeMux(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }
