package monitor

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	cryptox "composepilot/internal/crypto"
	"composepilot/internal/dockerops"
	"composepilot/internal/models"
	"composepilot/internal/notify"
	"composepilot/internal/store"
)

// WorkDirResolver resolves a stored service work dir to an absolute path.
type WorkDirResolver func(workDir string) string

type containerState struct {
	confirmed    string // "up" | "down"
	pending      string // candidate category not yet confirmed
	pendingCount int
}

type Monitor struct {
	store    *store.Store
	docker   *dockerops.Runner
	cipher   *cryptox.Cipher
	resolve  WorkDirResolver
	mu       sync.Mutex
	state    map[string]*containerState // key: service/container
	initDone map[int64]bool             // service id -> initial snapshot taken
}

func New(st *store.Store, docker *dockerops.Runner, cipher *cryptox.Cipher, resolve WorkDirResolver) *Monitor {
	return &Monitor{
		store:    st,
		docker:   docker,
		cipher:   cipher,
		resolve:  resolve,
		state:    make(map[string]*containerState),
		initDone: make(map[int64]bool),
	}
}

// Run starts the monitor polling loop until ctx is done.
func (m *Monitor) Run(ctx context.Context) {
	for {
		settings, err := m.store.GetMonitorSettings(ctx)
		if err != nil {
			log.Printf("monitor: load settings: %v", err)
			settings = models.MonitorSettings{Enabled: false, IntervalSeconds: 60}
		}
		interval := time.Duration(settings.IntervalSeconds) * time.Second
		if interval <= 0 {
			interval = 60 * time.Second
		}
		if settings.Enabled {
			m.tick(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// categorize maps a container to "up" or "down".
// "up"   : healthcheck "healthy" OR state "running" with no unhealthy check
// "down" : everything else (unhealthy, stopped, exited, unknown, ...)
func categorize(c models.Container) string {
	h := strings.ToLower(strings.TrimSpace(c.Health))
	if h == "healthy" {
		return "up"
	}
	if h == "unhealthy" {
		return "down"
	}
	if strings.Contains(strings.ToLower(c.State), "running") {
		return "up"
	}
	return "down"
}

func eventName(prev, curr string) string {
	if prev == "down" && curr == "up" {
		return "recovered"
	}
	if prev == "up" && curr == "down" {
		return "down"
	}
	return ""
}

func (m *Monitor) tick(ctx context.Context) {
	settings, err := m.store.GetMonitorSettings(ctx)
	if err != nil {
		log.Printf("monitor: load settings: %v", err)
		return
	}
	threshold := settings.ConfirmThreshold
	if threshold < 1 {
		threshold = 1
	}

	services, err := m.store.ListServices(ctx)
	if err != nil {
		log.Printf("monitor: list services: %v", err)
		return
	}
	targets, err := m.store.ListNotificationTargets(ctx)
	if err != nil {
		log.Printf("monitor: list targets: %v", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, svc := range services {
		workDir := svc.WorkDir
		if m.resolve != nil {
			workDir = m.resolve(svc.WorkDir)
		}
		scanCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		containers, err := m.docker.ListContainers(scanCtx, workDir, svc.ComposeFiles, svc.Environment)
		cancel()
		if err != nil {
			log.Printf("monitor: list containers for %s: %v", svc.Name, err)
			continue
		}
		first := !m.initDone[svc.ID]
		seen := make(map[string]bool, len(containers))
		for _, c := range containers {
			key := svc.Name + "/" + c.Name
			seen[key] = true
			curr := categorize(c)

			st, existed := m.state[key]
			if !existed || first {
				m.state[key] = &containerState{confirmed: curr}
				continue
			}

			if curr == st.confirmed {
				// current matches confirmed state; drop any pending flap
				st.pending = ""
				st.pendingCount = 0
				continue
			}

			// curr differs from confirmed — accumulate confirmation
			if st.pending == curr {
				st.pendingCount++
			} else {
				st.pending = curr
				st.pendingCount = 1
			}

			if st.pendingCount < threshold {
				continue
			}

			prev := st.confirmed
			st.confirmed = curr
			st.pending = ""
			st.pendingCount = 0

			ev := notify.Event{
				Service:   svc.Name,
				Container: c.Name,
				Prev:      prev,
				Curr:      curr,
				Event:     eventName(prev, curr),
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}
			ev.Message = notify.BuildMessage(ev)
			m.dispatchAll(ctx, targets, ev)
		}
		// forget containers that are gone so next reappearance is treated as new
		prefix := svc.Name + "/"
		for key := range m.state {
			if strings.HasPrefix(key, prefix) && !seen[key] {
				delete(m.state, key)
			}
		}
		m.initDone[svc.ID] = true
	}
}

func (m *Monitor) dispatchAll(ctx context.Context, targets []models.NotificationTarget, ev notify.Event) {
	for _, t := range targets {
		if !t.Enabled {
			continue
		}
		webhookURL, err := m.cipher.DecryptString(t.EncryptedWebhookURL)
		if err != nil {
			log.Printf("monitor: decrypt webhook %q: %v", t.Name, err)
			continue
		}
		tmpl := t.Template
		if strings.TrimSpace(tmpl) == "" {
			tmpl = notify.DefaultTemplate(t.Type)
		}
		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := notify.Dispatch(sendCtx, webhookURL, tmpl, ev); err != nil {
			log.Printf("monitor: dispatch to %q failed: %v", t.Name, err)
		}
		cancel()
	}
}
