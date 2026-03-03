//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dejanr/demo-it/internal/runctx"
)

func TestE2EStartAcceptsTranscriptFilePath(t *testing.T) {
	h := newE2EHarness(t)

	workspace := t.TempDir()
	transcriptPath := filepath.Join(workspace, "test.md")
	transcript := "```demo-it\n" +
		"title: File bootstrap\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: enter\n" +
		"```\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	env := setEnv(h.env, "TMUX", "1")
	if _, stderr, err := h.runDemoItWithEnv(env, transcriptPath); err != nil {
		t.Fatalf("start with transcript path failed: %v, stderr=%q", err, stderr)
	}

	demoSession := runctx.DemoSessionName(workspace)
	notesSession := runctx.NotesSessionName(workspace)

	demoExists, err := h.tmuxSessionExists(demoSession)
	if err != nil {
		t.Fatalf("check demo session: %v", err)
	}
	if !demoExists {
		t.Fatalf("expected demo session %q", demoSession)
	}
	notesExists, err := h.tmuxSessionExists(notesSession)
	if err != nil {
		t.Fatalf("check notes session: %v", err)
	}
	if !notesExists {
		t.Fatalf("expected notes session %q", notesSession)
	}

	if got := sessionOption(t, h, demoSession, "@demo_it_workspace"); got != workspace {
		t.Fatalf("@demo_it_workspace = %q, want %q", got, workspace)
	}
	if got := sessionOption(t, h, demoSession, "@demo_it_transcript"); got != transcriptPath {
		t.Fatalf("@demo_it_transcript = %q, want %q", got, transcriptPath)
	}
	if got := sessionOption(t, h, notesSession, "@demo_it_transcript"); got != transcriptPath {
		t.Fatalf("notes @demo_it_transcript = %q, want %q", got, transcriptPath)
	}
	if got := sessionOption(t, h, demoSession, "@demo_it_step"); got != "0" {
		t.Fatalf("@demo_it_step = %q, want 0", got)
	}
}
