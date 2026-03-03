//go:build e2e

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dejanr/demo-it/internal/daemon"
	"github.com/dejanr/demo-it/internal/runctx"
)

type e2eHarness struct {
	repoRoot   string
	cliPath    string
	socketPath string
	runID      string
	env        []string
	realTmux   string
	tmuxSocket string
	tmuxCmd    string
}

func TestE2ETraceNextTextSnapshot(t *testing.T) {
	h := newE2EHarness(t)

	workspace := t.TempDir()
	transcript := "```demo-it\n" +
		"title: Bootstrap placeholder\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: escape\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Send hi\n" +
		"actions:\n" +
		"  - kind: key-macro\n" +
		"    interval_ms: 10\n" +
		"    keys:\n" +
		"      - key: h\n" +
		"      - key: i\n" +
		"      - key: return\n" +
		"        delay_ms: 250\n" +
		"```\n"
	if err := os.WriteFile(filepath.Join(workspace, "demo-it.md"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	sessionName := runctx.DemoSessionName(workspace)
	h.createManagedDemoSession(t, sessionName, workspace, "sh", 0)
	h.mustRunTmux(t, "send-keys", "-t", sessionName, "stty -echo; cat", "Enter")
	time.Sleep(80 * time.Millisecond)

	_, _, err := h.runDemoIt("trace-next")
	if err != nil {
		t.Fatalf("trace-next failed: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(workspace, ".demo-it", "traces", "*.txt"))
	if err != nil {
		t.Fatalf("glob traces: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 trace txt file, got %d (%v)", len(matches), matches)
	}

	gotBytes, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read trace txt: %v", err)
	}
	got := string(gotBytes)

	wantBytes, err := os.ReadFile(filepath.Join("testdata", "trace-e2e-next.golden"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	want := string(wantBytes)
	if got != want {
		t.Fatalf("trace snapshot mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func newE2EHarness(t *testing.T) *e2eHarness {
	t.Helper()

	realTmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}

	repoRoot, err := runctx.RepoRoot()
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	binDir := t.TempDir()
	cliPath := filepath.Join(binDir, "demo-it")
	buildCmd := exec.Command("go", "build", "-o", cliPath, "./cmd/demo-it")
	buildCmd.Dir = repoRoot
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build demo-it: %v\n%s", err, string(output))
	}

	tmuxWrapperDir := t.TempDir()
	tmuxWrapperPath := filepath.Join(tmuxWrapperDir, "tmux")
	socketLabel := fmt.Sprintf("demo-it-e2e-%d", time.Now().UnixNano())
	tmuxTmpDir := t.TempDir()
	wrapper := "#!/bin/sh\n" +
		"exec \"$DEMO_IT_REAL_TMUX\" -L \"$DEMO_IT_TEST_TMUX_SOCKET\" -f /dev/null \"$@\"\n"
	if err := os.WriteFile(tmuxWrapperPath, []byte(wrapper), 0o755); err != nil {
		t.Fatalf("write tmux wrapper: %v", err)
	}

	socketPath := filepath.Join(t.TempDir(), "demo-it-e2e.sock")
	startDaemonForTest(t, socketPath)

	env := append([]string{}, os.Environ()...)
	env = setEnv(env, "DEMO_IT_REAL_TMUX", realTmux)
	env = setEnv(env, "DEMO_IT_TEST_TMUX_SOCKET", socketLabel)
	env = setEnv(env, "TMUX", "")
	env = setEnv(env, "TMUX_TMPDIR", tmuxTmpDir)
	env = setEnv(env, "PATH", tmuxWrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	h := &e2eHarness{
		repoRoot:   repoRoot,
		cliPath:    cliPath,
		socketPath: socketPath,
		runID:      "demo-it-e2e",
		env:        env,
		realTmux:   realTmux,
		tmuxSocket: socketLabel,
		tmuxCmd:    tmuxWrapperPath,
	}

	t.Cleanup(func() {
		cmd := exec.Command(h.realTmux, "-L", h.tmuxSocket, "-f", "/dev/null", "kill-server")
		cmd.Env = h.env
		_ = cmd.Run()
	})

	return h
}

func (h *e2eHarness) createManagedDemoSession(t *testing.T, sessionName string, workspace string, shellCommand string, step int) {
	t.Helper()
	h.createManagedSessionWithRole(t, sessionName, workspace, shellCommand, "demo", step)
}

func (h *e2eHarness) createManagedNotesSession(t *testing.T, sessionName string, workspace string, shellCommand string, step int) {
	t.Helper()
	h.createManagedSessionWithRole(t, sessionName, workspace, shellCommand, "notes", step)
}

func (h *e2eHarness) createManagedSessionWithRole(t *testing.T, sessionName string, workspace string, shellCommand string, role string, step int) {
	t.Helper()

	args := []string{"new-session", "-d", "-s", sessionName, "-c", workspace}
	if strings.TrimSpace(shellCommand) != "" {
		args = append(args, shellCommand)
	}
	h.mustRunTmux(t, args...)
	h.mustRunTmux(t, "set-option", "-t", sessionName, "-q", "@demo_it", "1")
	h.mustRunTmux(t, "set-option", "-t", sessionName, "-q", "@demo_it_workspace", workspace)
	h.mustRunTmux(t, "set-option", "-t", sessionName, "-q", "@demo_it_role", role)
	h.mustRunTmux(t, "set-option", "-t", sessionName, "-q", "@demo_it_step", strconv.Itoa(step))
}

func (h *e2eHarness) paneCount(t *testing.T, sessionName string) int {
	t.Helper()
	output := h.mustRunTmuxOutput(t, "list-panes", "-t", sessionName, "-F", "#{pane_id}")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func (h *e2eHarness) runDemoIt(args ...string) (string, string, error) {
	return h.runDemoItWithEnv(h.env, args...)
}

func (h *e2eHarness) runDemoItWithEnv(env []string, args ...string) (string, string, error) {
	cliArgs := append([]string{"--run-id", h.runID, "--socket", h.socketPath}, args...)
	cmd := exec.Command(h.cliPath, cliArgs...)
	cmd.Dir = h.repoRoot
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (h *e2eHarness) mustRunTmux(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command(h.tmuxCmd, args...)
	cmd.Env = h.env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tmux %v failed: %v\n%s", args, err, string(output))
	}
}

func (h *e2eHarness) mustRunTmuxOutput(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(h.tmuxCmd, args...)
	cmd.Env = h.env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v failed: %v\n%s", args, err, string(output))
	}
	return string(output)
}

func (h *e2eHarness) runTmuxNoFail(args ...string) error {
	_, err := h.tmuxOutput(args...)
	return err
}

func (h *e2eHarness) tmuxOutput(args ...string) (string, error) {
	cmd := exec.Command(h.tmuxCmd, args...)
	cmd.Env = h.env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %v failed: %w: %s", args, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (h *e2eHarness) tmuxSessionExists(sessionName string) (bool, error) {
	cmd := exec.Command(h.tmuxCmd, "has-session", "-t", sessionName)
	cmd.Env = h.env
	if output, err := cmd.CombinedOutput(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("tmux has-session %q failed: %w: %s", sessionName, err, strings.TrimSpace(string(output)))
	}
	return true, nil
}

func startDaemonForTest(t *testing.T, socketPath string) {
	t.Helper()
	service := daemon.NewService()
	server := daemon.NewServer(socketPath, service)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	waitForSocketPath(t, socketPath)

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("daemon server exited with error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for daemon server shutdown")
		}
	})
}

func waitForSocketPath(t *testing.T, socketPath string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket not created: %s", socketPath)
}

func setEnv(env []string, key string, value string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(env))
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		filtered = append(filtered, item)
	}
	return append(filtered, prefix+value)
}
