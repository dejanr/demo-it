//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dejanr/demo-it/internal/runctx"
)

func TestE2EKillAllPaneActionKeepsInitialPane(t *testing.T) {
	h := newE2EHarness(t)

	workspace := t.TempDir()
	transcript := "```demo-it\n" +
		"title: Bootstrap placeholder\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: escape\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Split first\n" +
		"actions:\n" +
		"  - kind: split-pane\n" +
		"    direction: right\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Split second\n" +
		"actions:\n" +
		"  - kind: split-pane\n" +
		"    direction: down\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Keep initial only\n" +
		"actions:\n" +
		"  - kind: killall-pane\n" +
		"```\n"
	if err := os.WriteFile(filepath.Join(workspace, "demo-it.md"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	sessionName := runctx.DemoSessionName(workspace)
	h.createManagedDemoSession(t, sessionName, workspace, "sh", 0)

	if _, stderr, err := h.runDemoIt("next"); err != nil {
		t.Fatalf("next (split first) failed: %v, stderr=%q", err, stderr)
	}
	if _, stderr, err := h.runDemoIt("next"); err != nil {
		t.Fatalf("next (split second) failed: %v, stderr=%q", err, stderr)
	}

	before := paneIDsByIndex(t, h, sessionName)
	if len(before) != 3 {
		t.Fatalf("pane count before killall = %d, want 3", len(before))
	}
	initialPane := before[0].id

	if _, stderr, err := h.runDemoIt("next"); err != nil {
		t.Fatalf("next (killall) failed: %v, stderr=%q", err, stderr)
	}

	after := paneIDsByIndex(t, h, sessionName)
	if len(after) != 1 {
		t.Fatalf("pane count after killall = %d, want 1", len(after))
	}
	if after[0].id != initialPane {
		t.Fatalf("remaining pane = %s, want initial pane %s", after[0].id, initialPane)
	}
}
