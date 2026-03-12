package models

import "time"

type Service struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	RepoURL         string            `json:"repoUrl"`
	Branch          string            `json:"branch"`
	WorkDir         string            `json:"workDir"`
	ComposeFiles    []string          `json:"composeFiles"`
	Environment     map[string]string `json:"environment"`
	ManagedFiles    []ManagedFile     `json:"managedFiles"`
	EncryptedSSHKey string            `json:"-"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

type ManagedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type JobRun struct {
	ID        int64     `json:"id"`
	ServiceID int64     `json:"serviceId,omitempty"`
	Action    string    `json:"action"`
	Status    string    `json:"status"`
	Output    string    `json:"output"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
}

type Container struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Service string `json:"service"`
}

type DockerNetwork struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}
