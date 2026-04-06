package dockerops

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"composepilot/internal/models"
)

type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Compose(ctx context.Context, workDir string, composeFiles []string, env map[string]string, args ...string) (string, error) {
	composeArgs := buildComposeArgs(composeFiles, args...)
	cmd := exec.CommandContext(ctx, "docker", composeArgs...)
	cmd.Dir = workDir
	cmd.Env = mergeEnv(env)
	output, err := runCommand(cmd, nil)
	if err != nil {
		return string(output), fmt.Errorf("docker %s: %w", strings.Join(composeArgs, " "), err)
	}
	return string(output), nil
}

func (r *Runner) ComposeStream(ctx context.Context, workDir string, composeFiles []string, env map[string]string, stream func(string), args ...string) (string, error) {
	composeArgs := buildComposeArgs(composeFiles, args...)
	cmd := exec.CommandContext(ctx, "docker", composeArgs...)
	cmd.Dir = workDir
	cmd.Env = mergeEnv(env)
	if stream != nil {
		stream("$ docker " + strings.Join(composeArgs, " ") + "\n")
	}
	output, err := runCommand(cmd, stream)
	if err != nil {
		return string(output), fmt.Errorf("docker %s: %w", strings.Join(composeArgs, " "), err)
	}
	return string(output), nil
}

func buildComposeArgs(composeFiles []string, args ...string) []string {
	result := []string{"compose"}
	for _, composeFile := range composeFiles {
		result = append(result, "-f", composeFile)
	}
	result = append(result, args...)
	return result
}

func mergeEnv(extra map[string]string) []string {
	env := os.Environ()
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}

func (r *Runner) ListContainers(ctx context.Context, workDir string, composeFiles []string, env map[string]string) ([]models.Container, error) {
	args := buildComposeArgs(composeFiles, "ps", "--format", "json")
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = workDir
	cmd.Env = mergeEnv(env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w: %s", err, output)
	}
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	containers := make([]models.Container, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var item struct {
			ID      string `json:"ID"`
			Name    string `json:"Name"`
			Image   string `json:"Image"`
			State   string `json:"State"`
			Status  string `json:"Status"`
			Service string `json:"Service"`
			Health  string `json:"Health"`
		}
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("parse compose ps: %w", err)
		}
		containers = append(containers, models.Container(item))
	}
	return containers, nil
}

func (r *Runner) ListNetworks(ctx context.Context) ([]models.DockerNetwork, error) {
	cmd := exec.CommandContext(ctx, "docker", "network", "ls", "--format", "{{json .}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker network ls: %w: %s", err, output)
	}
	var networks []models.DockerNetwork
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var item struct {
			ID     string `json:"ID"`
			Name   string `json:"Name"`
			Driver string `json:"Driver"`
			Scope  string `json:"Scope"`
		}
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("parse network: %w", err)
		}
		networks = append(networks, models.DockerNetwork(item))
	}
	return networks, scanner.Err()
}

func (r *Runner) CreateNetwork(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "network", "create", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("docker network create: %w", err)
	}
	return string(output), nil
}

func (r *Runner) RestartContainer(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "restart", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("docker restart: %w", err)
	}
	return string(output), nil
}

func (r *Runner) Logs(ctx context.Context, container string, tail int, follow bool) (*exec.Cmd, io.ReadCloser, error) {
	args := []string{"logs", fmt.Sprintf("--tail=%d", tail)}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, container)
	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start docker logs: %w", err)
	}
	return cmd, stdout, nil
}

func (r *Runner) LogsSnapshot(ctx context.Context, container string, tail int) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "logs", fmt.Sprintf("--tail=%d", tail), container)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("docker logs: %w", err)
	}
	return string(output), nil
}

func runCommand(cmd *exec.Cmd, stream func(string)) ([]byte, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var (
		out bytes.Buffer
		mu  sync.Mutex
		wg  sync.WaitGroup
	)
	copyStream := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				mu.Lock()
				out.Write(buf[:n])
				mu.Unlock()
				if stream != nil {
					stream(chunk)
				}
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go copyStream(stdout)
	go copyStream(stderr)
	wg.Wait()
	err = cmd.Wait()
	return out.Bytes(), err
}
