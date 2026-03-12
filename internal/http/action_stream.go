package httphandler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
)

type actionStreamMessage struct {
	Type   string `json:"type"`
	Action string `json:"action,omitempty"`
	Data   string `json:"data,omitempty"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	JobID  int64  `json:"jobId,omitempty"`
}

func (s *Server) handleServiceActionStream(w http.ResponseWriter, r *http.Request, id int64, action string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	svc, plainKey, err := s.loadService(r.Context(), id)
	if err != nil {
		s.writeError(w, statusForError(err), err)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade action stream websocket: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var writeMu sync.Mutex
	write := func(message actionStreamMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(message)
	}

	go func() {
		defer cancel()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	if err := write(actionStreamMessage{Type: "start", Action: action, Status: "running"}); err != nil {
		return
	}

	job, runErr := s.jobs.RunStreaming(ctx, id, action, func(chunk string) {
		if err := write(actionStreamMessage{Type: "output", Action: action, Data: chunk}); err != nil {
			cancel()
		}
	}, func(ctx context.Context, stream func(string)) (string, error) {
		workDir := s.serviceWorkDir(svc.WorkDir)
		switch action {
		case "deploy":
			return s.deployStream(ctx, svc, plainKey, stream)
		case "pull":
			output, err := s.git.CheckoutAndPullStream(ctx, workDir, svc.Branch, plainKey, stream)
			if err != nil {
				return output, err
			}
			chunk, err := s.materializeManagedFiles(ctx, svc, stream)
			return output + chunk, err
		case "build":
			return s.docker.ComposeStream(ctx, workDir, svc.ComposeFiles, svc.Environment, stream, "build")
		case "restart":
			return s.docker.ComposeStream(ctx, workDir, svc.ComposeFiles, svc.Environment, stream, "restart")
		case "stop":
			return s.docker.ComposeStream(ctx, workDir, svc.ComposeFiles, svc.Environment, stream, "stop")
		case "down":
			return s.docker.ComposeStream(ctx, workDir, svc.ComposeFiles, svc.Environment, stream, "down")
		default:
			return "", fmt.Errorf("unsupported action %q", action)
		}
	})
	done := actionStreamMessage{
		Type:   "done",
		Action: action,
		Status: job.Status,
		JobID:  job.ID,
	}
	if runErr != nil {
		done.Error = runErr.Error()
	}
	_ = write(done)
}
