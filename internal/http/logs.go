package httphandler

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	container := r.URL.Query().Get("container")
	if container == "" {
		s.writeError(w, http.StatusBadRequest, errors.New("container is required"))
		return
	}
	tail := parseIntDefault(r.URL.Query().Get("tail"), 200)
	follow := r.URL.Query().Get("follow") == "1"
	filter := r.URL.Query().Get("filter")
	if follow {
		cmd, reader, err := s.docker.Logs(r.Context(), container, tail, true)
		if err != nil {
			s.writeError(w, http.StatusBadGateway, err)
			return
		}
		defer func() {
			_ = reader.Close()
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			s.writeError(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
			return
		}
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			if filter != "" && !strings.Contains(line, filter) {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(line, "\n", " "))
			flusher.Flush()
		}
		return
	}
	output, err := s.docker.LogsSnapshot(r.Context(), container, tail)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, err)
		return
	}
	if filter != "" {
		var filtered []string
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, filter) {
				filtered = append(filtered, line)
			}
		}
		output = strings.Join(filtered, "\n")
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"output": output})
}
