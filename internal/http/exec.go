package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type execControlMessage struct {
	Type  string `json:"type"`
	Data  string `json:"data,omitempty"`
	Cols  uint16 `json:"cols,omitempty"`
	Rows  uint16 `json:"rows,omitempty"`
	Error string `json:"error,omitempty"`
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	container := r.URL.Query().Get("container")
	command := r.URL.Query().Get("cmd")
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	if container == "" {
		s.writeError(w, http.StatusBadRequest, errors.New("container is required"))
		return
	}
	if command == "" {
		command = "/bin/sh"
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade websocket: %v", err)
		return
	}
	defer conn.Close()
	args := []string{"exec", "-it", "-e", "TERM=xterm-256color", container}
	args = append(args, strings.Fields(command)...)
	cmd := exec.CommandContext(r.Context(), "docker", args...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		_ = conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}
	if cols > 0 && rows > 0 {
		_ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}
	defer func() {
		_ = ptmx.Close()
	}()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	var writeMu sync.Mutex
	writeMessage := func(messageType int, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(messageType, payload)
	}
	writeControl := func(message execControlMessage) error {
		payload, err := json.Marshal(message)
		if err != nil {
			return err
		}
		return writeMessage(websocket.TextMessage, payload)
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if writeErr := writeMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					cancel()
					return
				}
			}
			if err != nil {
				cancel()
				return
			}
		}
	}()
	go func() {
		err := cmd.Wait()
		message := execControlMessage{Type: "exit"}
		if err != nil {
			message.Error = err.Error()
		}
		_ = writeControl(message)
		cancel()
	}()
	for ctx.Err() == nil {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch messageType {
		case websocket.BinaryMessage:
			if _, err := ptmx.Write(data); err != nil {
				return
			}
		case websocket.TextMessage:
			var message execControlMessage
			if err := json.Unmarshal(data, &message); err != nil {
				if _, err := ptmx.Write(data); err != nil {
					return
				}
				continue
			}
			switch message.Type {
			case "input":
				if _, err := ptmx.Write([]byte(message.Data)); err != nil {
					return
				}
			case "resize":
				if message.Cols == 0 || message.Rows == 0 {
					continue
				}
				_ = pty.Setsize(ptmx, &pty.Winsize{Cols: message.Cols, Rows: message.Rows})
			}
		}
	}
}
