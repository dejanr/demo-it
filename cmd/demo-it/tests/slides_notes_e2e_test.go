//go:build e2e

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dejanr/demo-it/internal/runctx"
)

func TestE2ESlidesAndSpeakerNotesStayInSync(t *testing.T) {
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Skip("nvim not installed")
	}
	if _, err := exec.LookPath("base64"); err != nil {
		t.Skip("base64 not installed")
	}

	h := newE2EHarness(t)

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "slides"), 0o755); err != nil {
		t.Fatalf("mkdir slides: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "slides", "1-intro.md"), []byte("# intro\n"), 0o644); err != nil {
		t.Fatalf("write slide 1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "slides", "2-middle.md"), []byte("# middle\n"), 0o644); err != nil {
		t.Fatalf("write slide 2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "slides", "3-final.md"), []byte("# final\n"), 0o644); err != nil {
		t.Fatalf("write slide 3: %v", err)
	}

	transcript := "```demo-it\n" +
		"title: Bootstrap placeholder\n" +
		"actions:\n" +
		"  - kind: key\n" +
		"    key: return\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Intro slide\n" +
		"slide: slides/1-intro.md\n" +
		"speaker_notes: |\n" +
		"  NOTE_ONE\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Middle slide\n" +
		"slide: slides/2-middle.md\n" +
		"speaker_notes: |\n" +
		"  NOTE_TWO\n" +
		"```\n\n" +
		"```demo-it\n" +
		"title: Final slide\n" +
		"slide: slides/3-final.md\n" +
		"speaker_notes: |\n" +
		"  NOTE_THREE\n" +
		"```\n"
	if err := os.WriteFile(filepath.Join(workspace, "demo-it.md"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	demoSession := runctx.DemoSessionName(workspace)
	notesSession := runctx.NotesSessionName(workspace)
	h.createManagedDemoSession(t, demoSession, workspace, "sh", 0)
	h.createManagedNotesSession(t, notesSession, workspace, "sh", 0)

	runAndParseState(t, h, "next")
	state := runAndParseState(t, h, "run-status")
	if got := sessionOption(t, h, demoSession, "@demo_it_slide"); got != "slides/1-intro.md" {
		t.Fatalf("slide option after step 1 = %q, want %q", got, "slides/1-intro.md")
	}
	if got := state["speaker_notes"]; got != "NOTE_ONE\n\n---\nNext: Middle slide\n" {
		t.Fatalf("speaker_notes step 1 = %q", got)
	}
	waitForPaneContains(t, h, notesSession, "NOTE_ONE", 3*time.Second)

	runAndParseState(t, h, "next")
	state = runAndParseState(t, h, "run-status")
	if got := sessionOption(t, h, demoSession, "@demo_it_slide"); got != "slides/2-middle.md" {
		t.Fatalf("slide option after step 2 = %q, want %q", got, "slides/2-middle.md")
	}
	if got := state["speaker_notes"]; got != "NOTE_TWO\n\n---\nNext: Final slide\n" {
		t.Fatalf("speaker_notes step 2 = %q", got)
	}
	waitForPaneContains(t, h, notesSession, "NOTE_TWO", 3*time.Second)

	runAndParseState(t, h, "next")
	state = runAndParseState(t, h, "run-status")
	if got := sessionOption(t, h, demoSession, "@demo_it_slide"); got != "slides/3-final.md" {
		t.Fatalf("slide option after step 3 = %q, want %q", got, "slides/3-final.md")
	}
	if got := state["speaker_notes"]; got != "NOTE_THREE\n" {
		t.Fatalf("speaker_notes step 3 = %q", got)
	}
	waitForPaneContains(t, h, notesSession, "NOTE_THREE", 3*time.Second)
}

func runAndParseState(t *testing.T, h *e2eHarness, command string) map[string]string {
	t.Helper()
	stdout, stderr, err := h.runDemoIt(command)
	if err != nil {
		t.Fatalf("%s failed: %v, stderr=%q", command, err, stderr)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("decode %s response: %v, stdout=%q", command, err, stdout)
	}
	out := map[string]string{}
	for key, value := range decoded {
		stringValue, ok := value.(string)
		if ok {
			out[key] = stringValue
		}
	}
	return out
}

func sessionOption(t *testing.T, h *e2eHarness, sessionName string, option string) string {
	t.Helper()
	output := h.mustRunTmuxOutput(t, "show-options", "-qv", "-t", sessionName, option)
	return strings.TrimSpace(output)
}

func waitForPaneContains(t *testing.T, h *e2eHarness, sessionName string, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		output := h.mustRunTmuxOutput(t, "capture-pane", "-p", "-t", sessionName)
		if strings.Contains(output, expected) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	output := h.mustRunTmuxOutput(t, "capture-pane", "-p", "-t", sessionName)
	t.Fatalf("expected notes pane to contain %q, got %q", expected, output)
}
