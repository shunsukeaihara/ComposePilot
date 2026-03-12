package gitops

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) EnsureCloned(ctx context.Context, repoURL, branch, targetDir, privateKey string) (string, error) {
	if _, err := os.Stat(filepath.Join(targetDir, ".git")); err == nil {
		return r.CheckoutAndPull(ctx, targetDir, branch, privateKey)
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return "", fmt.Errorf("create parent dir: %w", err)
	}
	keyPath, cleanup, err := writeKey(privateKey)
	if err != nil {
		return "", err
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, repoURL, targetDir)
	cmd.Env = append(os.Environ(), sshCommandEnv(keyPath))
	output, err := runCommand(cmd, nil)
	if err != nil {
		return string(output), fmt.Errorf("git clone: %w", err)
	}
	return string(output), nil
}

func (r *Runner) EnsureClonedStream(ctx context.Context, repoURL, branch, targetDir, privateKey string, stream func(string)) (string, error) {
	if _, err := os.Stat(filepath.Join(targetDir, ".git")); err == nil {
		return r.CheckoutAndPullStream(ctx, targetDir, branch, privateKey, stream)
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return "", fmt.Errorf("create parent dir: %w", err)
	}
	keyPath, cleanup, err := writeKey(privateKey)
	if err != nil {
		return "", err
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, repoURL, targetDir)
	cmd.Env = append(os.Environ(), sshCommandEnv(keyPath))
	if stream != nil {
		stream("$ git clone --branch " + branch + " " + repoURL + " " + targetDir + "\n")
	}
	output, err := runCommand(cmd, stream)
	if err != nil {
		return string(output), fmt.Errorf("git clone: %w", err)
	}
	return string(output), nil
}

func (r *Runner) CheckoutAndPull(ctx context.Context, targetDir, branch, privateKey string) (string, error) {
	return r.CheckoutAndPullStream(ctx, targetDir, branch, privateKey, nil)
}

func (r *Runner) CheckoutAndPullStream(ctx context.Context, targetDir, branch, privateKey string, stream func(string)) (string, error) {
	keyPath, cleanup, err := writeKey(privateKey)
	if err != nil {
		return "", err
	}
	defer cleanup()

	steps := [][]string{
		{"fetch", "origin", branch},
		{"checkout", branch},
		{"pull", "--ff-only", "origin", branch},
	}
	var output strings.Builder
	for _, args := range steps {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = targetDir
		cmd.Env = append(os.Environ(), sshCommandEnv(keyPath))
		if stream != nil {
			stream("$ git " + strings.Join(args, " ") + "\n")
		}
		chunk, err := runCommand(cmd, stream)
		output.Write(chunk)
		if err == nil {
			continue
		}
		if len(args) >= 2 && args[0] == "checkout" {
			track := exec.CommandContext(ctx, "git", "checkout", "-B", branch, "--track", "origin/"+branch)
			track.Dir = targetDir
			track.Env = append(os.Environ(), sshCommandEnv(keyPath))
			if stream != nil {
				stream("$ git checkout -B " + branch + " --track origin/" + branch + "\n")
			}
			chunk, trackErr := runCommand(track, stream)
			output.Write(chunk)
			if trackErr == nil {
				continue
			}
		}
		return output.String(), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return output.String(), nil
}

func writeKey(privateKey string) (string, func(), error) {
	privateKey = normalizePrivateKey(privateKey)
	file, err := os.CreateTemp("", "composepilot-key-*")
	if err != nil {
		return "", nil, fmt.Errorf("create key file: %w", err)
	}
	if _, err := file.WriteString(privateKey); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("write key file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("chmod key file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("close key file: %w", err)
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}

func normalizePrivateKey(privateKey string) string {
	privateKey = strings.ReplaceAll(privateKey, "\r\n", "\n")
	privateKey = strings.ReplaceAll(privateKey, "\r", "\n")
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" {
		return ""
	}
	return privateKey + "\n"
}

func sshCommandEnv(keyPath string) string {
	return fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new", keyPath)
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
