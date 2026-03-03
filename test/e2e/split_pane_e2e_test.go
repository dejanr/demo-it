//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dejanr/demo-it/internal/runctx"
)

func TestE2ESplitPaneActionOpensExtraPane(t *testing.T) {
	h := newE2EHarness(t)

	workspace := t.TempDir()
	transcript := "```demo-it\n" +
		"title: Bootstrap placeholder\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: escape\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Split right\n" +
		"actions:\n" +
		"  - kind: split-pane\n" +
		"    direction: right\n" +
		"```\n"
	if err := os.WriteFile(filepath.Join(workspace, "demo-it.md"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	sessionName := runctx.DemoSessionName(workspace)
	h.createManagedDemoSession(t, sessionName, workspace, "sh", 0)

	before := h.paneCount(t, sessionName)
	if before != 1 {
		t.Fatalf("pane count before next = %d, want 1", before)
	}

	_, stderr, err := h.runDemoIt("next")
	if err != nil {
		t.Fatalf("next failed: %v, stderr=%q", err, stderr)
	}

	after := h.paneCount(t, sessionName)
	if after != 2 {
		t.Fatalf("pane count after split action = %d, want 2", after)
	}
}
