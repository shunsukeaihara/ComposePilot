package httphandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleNetworks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		networks, err := s.docker.ListNetworks(r.Context())
		if err != nil {
			s.writeError(w, http.StatusBadGateway, err)
			return
		}
		s.writeJSON(w, http.StatusOK, networks)
	case http.MethodPost:
		var req networkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			s.writeError(w, http.StatusBadRequest, errors.New("network name is required"))
			return
		}
		output, err := s.docker.CreateNetwork(r.Context(), req.Name)
		if err != nil {
			s.writeError(w, http.StatusBadGateway, fmt.Errorf("%s: %w", strings.TrimSpace(output), err))
			return
		}
		s.writeJSON(w, http.StatusCreated, map[string]string{"output": strings.TrimSpace(output)})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
