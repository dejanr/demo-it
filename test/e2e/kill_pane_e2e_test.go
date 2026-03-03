//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dejanr/demo-it/internal/runctx"
)

func TestE2EKillPaneActionClosesLatestOpenedPane(t *testing.T) {
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
		"title: Kill latest\n" +
		"actions:\n" +
		"  - kind: kill-pane\n" +
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
		t.Fatalf("pane count before close = %d, want 3", len(before))
	}
	latest := before[len(before)-1].id

	if _, stderr, err := h.runDemoIt("next"); err != nil {
		t.Fatalf("next (close latest) failed: %v, stderr=%q", err, stderr)
	}

	after := paneIDsByIndex(t, h, sessionName)
	if len(after) != 2 {
		t.Fatalf("pane count after close = %d, want 2", len(after))
	}
	for _, pane := range after {
		if pane.id == latest {
			t.Fatalf("latest pane %s still exists after kill-pane", latest)
		}
	}
}

type paneRef struct {
	index int
	id    string
}

func paneIDsByIndex(t *testing.T, h *e2eHarness, sessionName string) []paneRef {
	t.Helper()
	output := h.mustRunTmuxOutput(t, "list-panes", "-t", sessionName, "-F", "#{pane_index}\t#{pane_id}")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	refs := make([]paneRef, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected list-panes line: %q", line)
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			t.Fatalf("parse pane index from %q: %v", line, err)
		}
		refs = append(refs, paneRef{index: idx, id: strings.TrimSpace(parts[1])})
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].index < refs[j].index
	})
	return refs
}
