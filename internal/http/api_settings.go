package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"composepilot/internal/models"
	"composepilot/internal/notify"
)

func (s *Server) handleMonitoringPage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.renderMonitoringPage(w, r, "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		enabled := r.FormValue("enabled") == "on"
		interval, _ := strconv.Atoi(r.FormValue("intervalSeconds"))
		if interval <= 0 {
			interval = 60
		}
		threshold, _ := strconv.Atoi(r.FormValue("confirmThreshold"))
		if threshold < 1 {
			threshold = 2
		}
		if err := s.store.SaveMonitorSettings(r.Context(), models.MonitorSettings{Enabled: enabled, IntervalSeconds: interval, ConfirmThreshold: threshold}); err != nil {
			s.writeError(w, http.StatusInternalServerError, err)
			return
		}
		s.renderMonitoringPage(w, r, "Monitoring settings saved.")
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNotificationTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	action := r.FormValue("action")
	switch action {
	case "create":
		s.createNotificationTarget(w, r)
	case "update":
		s.updateNotificationTarget(w, r)
	case "delete":
		s.deleteNotificationTarget(w, r)
	case "test":
		s.testNotificationTarget(w, r)
	default:
		s.writeError(w, http.StatusBadRequest, fmt.Errorf("unknown action"))
	}
}

func (s *Server) createNotificationTarget(w http.ResponseWriter, r *http.Request) {
	t, err := s.targetFromForm(r)
	if err != nil {
		s.renderMonitoringPage(w, r, err.Error())
		return
	}
	if t.Template == "" {
		t.Template = notify.DefaultTemplate(t.Type)
	}
	enc, err := s.cipher.EncryptString(t.WebhookURL)
	if err != nil {
		s.renderMonitoringPage(w, r, err.Error())
		return
	}
	t.EncryptedWebhookURL = enc
	t.WebhookURL = ""
	if _, err := s.store.CreateNotificationTarget(r.Context(), t); err != nil {
		s.renderMonitoringPage(w, r, err.Error())
		return
	}
	s.renderMonitoringPage(w, r, "Notification target created.")
}

func (s *Server) updateNotificationTarget(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	existing, err := s.store.GetNotificationTarget(r.Context(), id)
	if err != nil {
		s.writeError(w, statusForError(err), err)
		return
	}
	t, err := s.targetFromForm(r)
	if err != nil {
		s.renderMonitoringPage(w, r, err.Error())
		return
	}
	t.ID = id
	if t.Template == "" {
		t.Template = notify.DefaultTemplate(t.Type)
	}
	// If webhook URL left blank, preserve existing encrypted value.
	if strings.TrimSpace(t.WebhookURL) == "" {
		t.EncryptedWebhookURL = existing.EncryptedWebhookURL
	} else {
		enc, err := s.cipher.EncryptString(t.WebhookURL)
		if err != nil {
			s.renderMonitoringPage(w, r, err.Error())
			return
		}
		t.EncryptedWebhookURL = enc
	}
	t.WebhookURL = ""
	if _, err := s.store.UpdateNotificationTarget(r.Context(), t); err != nil {
		s.renderMonitoringPage(w, r, err.Error())
		return
	}
	s.renderMonitoringPage(w, r, "Notification target updated.")
}

func (s *Server) deleteNotificationTarget(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteNotificationTarget(r.Context(), id); err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.renderMonitoringPage(w, r, "Notification target deleted.")
}

func (s *Server) testNotificationTarget(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	target, err := s.store.GetNotificationTarget(r.Context(), id)
	if err != nil {
		s.writeError(w, statusForError(err), err)
		return
	}
	webhookURL, err := s.cipher.DecryptString(target.EncryptedWebhookURL)
	if err != nil {
		s.renderMonitoringPage(w, r, fmt.Sprintf("Decrypt webhook failed: %v", err))
		return
	}
	tmpl := target.Template
	if strings.TrimSpace(tmpl) == "" {
		tmpl = notify.DefaultTemplate(target.Type)
	}
	ev := notify.Event{
		Service:   "composepilot",
		Container: "test",
		Event:     "test",
		Prev:      "running",
		Curr:      "running",
	}
	ev.Message = "[ComposePilot] test notification from " + target.Name
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := notify.Dispatch(ctx, webhookURL, tmpl, ev); err != nil {
		s.renderMonitoringPage(w, r, fmt.Sprintf("Test failed: %v", err))
		return
	}
	s.renderMonitoringPage(w, r, "Test notification sent.")
}

func (s *Server) targetFromForm(r *http.Request) (models.NotificationTarget, error) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return models.NotificationTarget{}, fmt.Errorf("name is required")
	}
	typ := strings.TrimSpace(r.FormValue("type"))
	switch typ {
	case models.NotificationTypeDiscord, models.NotificationTypeSlack, models.NotificationTypeZapier:
	default:
		return models.NotificationTarget{}, fmt.Errorf("invalid type")
	}
	return models.NotificationTarget{
		Name:       name,
		Type:       typ,
		WebhookURL: strings.TrimSpace(r.FormValue("webhookUrl")),
		Template:   r.FormValue("template"),
		Enabled:    r.FormValue("enabled") == "on",
	}, nil
}

func (s *Server) renderMonitoringPage(w http.ResponseWriter, r *http.Request, flash string) {
	data, err := s.newViewData(r, "content-monitoring")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	settings, err := s.store.GetMonitorSettings(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	targets, err := s.store.ListNotificationTargets(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err)
		return
	}
	data.MonitorSettings = &settings
	data.NotificationTargets = targets
	data.DiscordTemplate = notify.DefaultTemplate(models.NotificationTypeDiscord)
	data.SlackTemplate = notify.DefaultTemplate(models.NotificationTypeSlack)
	data.ZapierTemplate = notify.DefaultTemplate(models.NotificationTypeZapier)
	if flash != "" {
		data.Flash = flash
	}
	s.renderTemplate(w, r, data)
}
