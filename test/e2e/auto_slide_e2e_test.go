//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dejanr/demo-it/internal/runctx"
)

func TestE2EAutoSlideInMSAdvancesToNextStep(t *testing.T) {
	h := newE2EHarness(t)

	workspace := t.TempDir()
	transcript := "```demo-it\n" +
		"title: Bootstrap placeholder\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: return\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Auto\n" +
		"auto_slide_in_ms: 200\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: return\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Last\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: return\n" +
		"```\n"
	if err := os.WriteFile(filepath.Join(workspace, "demo-it.md"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	sessionName := runctx.DemoSessionName(workspace)
	h.createManagedDemoSession(t, sessionName, workspace, "sh", 0)

	if _, stderr, err := h.runDemoIt("next"); err != nil {
		t.Fatalf("next failed: %v, stderr=%q", err, stderr)
	}

	waitForSessionStep(t, h, sessionName, "2", 2*time.Second)
	time.Sleep(350 * time.Millisecond)
	if got := sessionStep(t, h, sessionName); got != "2" {
		t.Fatalf("expected step to remain 2, got %s", got)
	}
}

func TestE2EAutoSlideInMSChainsToEndOfTranscript(t *testing.T) {
	h := newE2EHarness(t)

	workspace := t.TempDir()
	transcript := "```demo-it\n" +
		"title: Auto first\n" +
		"auto_slide_in_ms: 150\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: return\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Auto second\n" +
		"auto_slide_in_ms: 150\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: return\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Last\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: return\n" +
		"```\n"
	if err := os.WriteFile(filepath.Join(workspace, "demo-it.md"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	sessionName := runctx.DemoSessionName(workspace)
	h.createManagedDemoSession(t, sessionName, workspace, "sh", -1)

	if _, stderr, err := h.runDemoIt("next"); err != nil {
		t.Fatalf("next failed: %v, stderr=%q", err, stderr)
	}

	waitForSessionStep(t, h, sessionName, "2", 3*time.Second)
	time.Sleep(300 * time.Millisecond)
	if got := sessionStep(t, h, sessionName); got != "2" {
		t.Fatalf("expected step to remain at last step, got %s", got)
	}
}

func waitForSessionStep(t *testing.T, h *e2eHarness, sessionName string, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if step := sessionStep(t, h, sessionName); step == expected {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected step %s, got %s", expected, sessionStep(t, h, sessionName))
}

func sessionStep(t *testing.T, h *e2eHarness, sessionName string) string {
	t.Helper()
	output := h.mustRunTmuxOutput(t, "show-options", "-v", "-t", sessionName, "@demo_it_step")
	return strings.TrimSpace(output)
}
