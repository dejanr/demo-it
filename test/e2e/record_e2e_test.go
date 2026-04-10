//go:build e2e

package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dejanr/demo-it/internal/runctx"
)

type asyncRecordProcess struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
	done   chan error
}

func TestE2ERecordEchoHelloSnapshot(t *testing.T) {
	h := newE2EHarness(t)
	workspace := t.TempDir()
	sessionName := runctx.SessionPrefix(workspace) + "-record"

	record := startRecordProcess(t, h, workspace)
	waitForRecordSessionReady(t, h, sessionName, 4*time.Second)

	h.mustRunTmux(t, "send-keys", "-t", sessionName, "echo hello", "Enter")
	time.Sleep(250 * time.Millisecond)
	h.mustRunTmux(t, "send-keys", "-t", sessionName, "C-d")

	waitForRecordProcess(t, h, sessionName, record, 6*time.Second)
	assertSnapshot(t, "record-echo-hello.golden", record.stdout.String())
}

func TestE2ERecordSplitRightAndDoubleCtrlDSnapshot(t *testing.T) {
	h := newE2EHarness(t)
	workspace := t.TempDir()
	sessionName := runctx.SessionPrefix(workspace) + "-record"

	record := startRecordProcess(t, h, workspace)
	waitForRecordSessionReady(t, h, sessionName, 4*time.Second)

	h.mustRunTmux(t, "split-window", "-t", sessionName, "-h")
	waitForRecordedEventKind(t, workspace, "split-pane", 3*time.Second)
	if panes := h.paneCount(t, sessionName); panes != 2 {
		t.Fatalf("expected 2 panes after split, got %d", panes)
	}
	time.Sleep(250 * time.Millisecond)
	h.mustRunTmux(t, "send-keys", "-t", sessionName, "C-d")
	time.Sleep(150 * time.Millisecond)
	h.mustRunTmux(t, "send-keys", "-t", sessionName, "C-d")

	waitForRecordProcess(t, h, sessionName, record, 6*time.Second)
	assertSnapshot(t, "record-split-right-double-ctrl-d.golden", record.stdout.String())
}

func TestE2ERecordStdoutCanBeRedirectedToFile(t *testing.T) {
	h := newE2EHarness(t)
	workspace := t.TempDir()
	sessionName := runctx.SessionPrefix(workspace) + "-record"
	outputPath := filepath.Join(workspace, "redirected.md")
	outputFile, err := os.Create(outputPath)
	if err != nil {
		t.Fatalf("create redirected output file: %v", err)
	}
	t.Cleanup(func() { _ = outputFile.Close() })

	record := startRecordProcessWithOutput(t, h, outputFile, workspace)
	waitForRecordSessionReady(t, h, sessionName, 4*time.Second)

	h.mustRunTmux(t, "send-keys", "-t", sessionName, "echo hello", "Enter")
	time.Sleep(250 * time.Millisecond)
	h.mustRunTmux(t, "send-keys", "-t", sessionName, "C-d")

	waitForRecordProcess(t, h, sessionName, record, 6*time.Second)
	if err := outputFile.Close(); err != nil {
		t.Fatalf("close redirected output file: %v", err)
	}

	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read redirected output: %v", err)
	}
	assertSnapshot(t, "record-echo-hello.golden", string(contents))
}

func TestE2ERecordWritesOutputFileFlag(t *testing.T) {
	h := newE2EHarness(t)
	workspace := t.TempDir()
	sessionName := runctx.SessionPrefix(workspace) + "-record"
	outputPath := filepath.Join(workspace, "via-flag.md")

	record := startRecordProcess(t, h, "-f", outputPath, workspace)
	waitForRecordSessionReady(t, h, sessionName, 4*time.Second)

	h.mustRunTmux(t, "send-keys", "-t", sessionName, "echo hello", "Enter")
	time.Sleep(250 * time.Millisecond)
	h.mustRunTmux(t, "send-keys", "-t", sessionName, "C-d")

	waitForRecordProcess(t, h, sessionName, record, 6*time.Second)
	if got := strings.TrimSpace(record.stdout.String()); got != "" {
		t.Fatalf("expected no stdout when -f is used, got %q", got)
	}

	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read -f output: %v", err)
	}
	assertSnapshot(t, "record-echo-hello.golden", string(contents))
}

func TestE2ERecordFailsClearlyWhenShellExitsDuringSetup(t *testing.T) {
	h := newE2EHarness(t)
	workspace := t.TempDir()
	exitShell := filepath.Join(t.TempDir(), "exit-shell.sh")
	if err := os.WriteFile(exitShell, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write exit shell: %v", err)
	}

	record := startRecordProcessWithEnv(t, h, nil, []string{"SHELL=" + exitShell}, workspace)
	defer func() {
		if record.cmd.Process != nil {
			_ = record.cmd.Process.Kill()
		}
	}()

	select {
	case err := <-record.done:
		if err == nil {
			t.Fatal("expected record command to fail")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("timed out waiting for record setup failure")
	}

	stderr := record.stderr.String()
	if !strings.Contains(stderr, "record shell for") || !strings.Contains(stderr, "exited during setup") {
		t.Fatalf("expected clear setup failure, got stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, "set record kill-pane hook") {
		t.Fatalf("expected setup failure after hooks are installed, got stderr:\n%s", stderr)
	}
}

func startRecordProcess(t *testing.T, h *e2eHarness, args ...string) *asyncRecordProcess {
	return startRecordProcessWithOutput(t, h, nil, args...)
}

func startRecordProcessWithOutput(t *testing.T, h *e2eHarness, stdout io.Writer, args ...string) *asyncRecordProcess {
	return startRecordProcessWithEnv(t, h, stdout, nil, args...)
}

func startRecordProcessWithEnv(t *testing.T, h *e2eHarness, stdout io.Writer, env []string, args ...string) *asyncRecordProcess {
	t.Helper()
	cliArgs := append([]string{"--run-id", h.runID, "--socket", h.socketPath, "record", "--yes"}, args...)
	cmd := exec.Command(h.cliPath, cliArgs...)
	cmd.Dir = h.repoRoot
	cmd.Env = setEnv(h.env, "TMUX", "1")
	for _, entry := range env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid env override %q", entry)
		}
		cmd.Env = setEnv(cmd.Env, parts[0], parts[1])
	}

	process := &asyncRecordProcess{cmd: cmd, done: make(chan error, 1)}
	if stdout != nil {
		cmd.Stdout = stdout
	} else {
		cmd.Stdout = &process.stdout
	}
	cmd.Stderr = &process.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start record command: %v", err)
	}
	go func() {
		process.done <- cmd.Wait()
	}()
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	return process
}

func waitForRecordSessionReady(t *testing.T, h *e2eHarness, sessionName string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		exists, err := h.tmuxSessionExists(sessionName)
		if err != nil || !exists {
			time.Sleep(25 * time.Millisecond)
			continue
		}

		hooksOutput, err := h.tmuxOutput("show-hooks", "-t", sessionName)
		if err != nil {
			time.Sleep(25 * time.Millisecond)
			continue
		}
		if strings.Contains(hooksOutput, "after-split-window") && strings.Contains(hooksOutput, "__record-split-event") {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for record session %q hooks", sessionName)
}

func waitForRecordProcess(t *testing.T, h *e2eHarness, sessionName string, process *asyncRecordProcess, timeout time.Duration) {
	t.Helper()
	select {
	case err := <-process.done:
		if err != nil {
			t.Fatalf("record command failed: %v\nstderr:\n%s\nstdout:\n%s", err, process.stderr.String(), process.stdout.String())
		}
	case <-time.After(timeout):
		_ = h.runTmuxNoFail("kill-session", "-t", sessionName)
		t.Fatalf("record command timed out\nstderr:\n%s\nstdout:\n%s", process.stderr.String(), process.stdout.String())
	}
}

func waitForRecordedEventKind(t *testing.T, workspace string, kind string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob(filepath.Join(workspace, ".demo-it", "recordings", "*", "events.ndjson"))
		if len(matches) > 0 {
			latest := matches[len(matches)-1]
			bytes, err := os.ReadFile(latest)
			if err == nil && strings.Contains(string(bytes), `"kind":"`+kind+`"`) {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for recorded event kind %q", kind)
}

func assertSnapshot(t *testing.T, fileName string, got string) {
	t.Helper()
	path := filepath.Join("testdata", fileName)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	want := string(wantBytes)
	if got != want {
		t.Fatalf("snapshot mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", fileName, got, want)
	}
}
