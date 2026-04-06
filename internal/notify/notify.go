package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"text/template"
	"time"

	"composepilot/internal/models"
)

// Event is the payload passed to a notification template.
type Event struct {
	Service      string
	Container    string
	Event        string // e.g. "down", "recovered", "unhealthy", "healthy", "changed"
	Prev         string
	Curr         string
	Timestamp    string
	Message      string
	Level        string // "good" | "warning" | "danger"
	SlackColor   string // "good" | "warning" | "danger"
	DiscordColor int    // decimal color for Discord embeds
}

// ClassifyLevel derives a severity level from an event name.
func ClassifyLevel(eventName string) string {
	switch eventName {
	case "recovered", "healthy":
		return "good"
	case "down", "unhealthy":
		return "danger"
	default:
		return "warning"
	}
}

// DiscordColorFor returns the Discord embed color for a level.
func DiscordColorFor(level string) int {
	switch level {
	case "good":
		return 3066993 // green
	case "danger":
		return 15158332 // red
	default:
		return 15844367 // amber
	}
}

// DefaultTemplate returns a default JSON-body template for the given target type.
func DefaultTemplate(targetType string) string {
	switch targetType {
	case models.NotificationTypeDiscord:
		return `{"embeds":[{"title":"ComposePilot","description":{{js .Message}},"color":{{.DiscordColor}}}]}`
	case models.NotificationTypeSlack:
		return `{"attachments":[{"color":{{js .SlackColor}},"text":{{js .Message}}}]}`
	case models.NotificationTypeZapier:
		return `{"service": {{js .Service}}, "container": {{js .Container}}, "event": {{js .Event}}, "level": {{js .Level}}, "prev": {{js .Prev}}, "curr": {{js .Curr}}, "timestamp": {{js .Timestamp}}, "message": {{js .Message}}}`
	default:
		return `{"text": {{js .Message}}}`
	}
}

// BuildMessage builds the default human-readable message.
func BuildMessage(ev Event) string {
	return fmt.Sprintf("[ComposePilot] %s/%s: %s → %s (%s)", ev.Service, ev.Container, ev.Prev, ev.Curr, ev.Event)
}

var funcs = template.FuncMap{
	"js": func(v any) (string, error) {
		b, err := json.Marshal(fmt.Sprintf("%v", v))
		if err != nil {
			return "", err
		}
		return string(b), nil
	},
}

// Render renders the given template with the event and returns the JSON body.
func Render(tmplText string, ev Event) ([]byte, error) {
	if ev.Message == "" {
		ev.Message = BuildMessage(ev)
	}
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if ev.Level == "" {
		ev.Level = ClassifyLevel(ev.Event)
	}
	if ev.SlackColor == "" {
		ev.SlackColor = ev.Level
	}
	if ev.DiscordColor == 0 {
		ev.DiscordColor = DiscordColorFor(ev.Level)
	}
	tmpl, err := template.New("notify").Funcs(funcs).Parse(tmplText)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ev); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	// Validate JSON.
	var dummy any
	if err := json.Unmarshal(buf.Bytes(), &dummy); err != nil {
		return nil, fmt.Errorf("rendered template is not valid JSON: %w", err)
	}
	return buf.Bytes(), nil
}

// Send POSTs a rendered body to a webhook URL.
func Send(ctx context.Context, webhookURL string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook %s returned %d: %s", webhookURL, resp.StatusCode, string(snippet))
	}
	return nil
}

// Dispatch renders and sends an event to a target.
func Dispatch(ctx context.Context, webhookURL, tmplText string, ev Event) error {
	body, err := Render(tmplText, ev)
	if err != nil {
		return err
	}
	return Send(ctx, webhookURL, body)
}
