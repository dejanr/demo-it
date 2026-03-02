package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dejanr/demo-it/internal/transcript"
)

func TestTmuxKeyNormalizesReturn(t *testing.T) {
	if got := tmuxKey("return"); got != "Enter" {
		t.Fatalf("tmuxKey(return) = %q, want Enter", got)
	}
	if got := tmuxKey("Enter"); got != "Enter" {
		t.Fatalf("tmuxKey(Enter) = %q, want Enter", got)
	}
}

func TestTmuxKeyNormalizesCommonSpecialKeys(t *testing.T) {
	if got := tmuxKey("escape"); got != "Escape" {
		t.Fatalf("tmuxKey(escape) = %q, want Escape", got)
	}
	if got := tmuxKey("space"); got != "Space" {
		t.Fatalf("tmuxKey(space) = %q, want Space", got)
	}
	if got := tmuxKey("tab"); got != "Tab" {
		t.Fatalf("tmuxKey(tab) = %q, want Tab", got)
	}
}

func TestResolveCLIPathUsesPathDemoItWhenOverrideEmpty(t *testing.T) {
	temp := t.TempDir()
	demoItPath := filepath.Join(temp, "demo-it")
	if err := os.WriteFile(demoItPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write demo-it: %v", err)
	}

	t.Setenv("DEMO_IT_PATH", "")
	t.Setenv("PATH", temp)

	resolved, err := resolveCLIPath()
	if err != nil {
		t.Fatalf("resolveCLIPath error: %v", err)
	}
	if resolved != demoItPath {
		t.Fatalf("resolveCLIPath = %q, want %q", resolved, demoItPath)
	}
}

func TestResolveCLIPathUsesOverride(t *testing.T) {
	temp := t.TempDir()
	custom := filepath.Join(temp, "demo-it-local")
	if err := os.WriteFile(custom, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write custom demo-it: %v", err)
	}

	t.Setenv("DEMO_IT_PATH", custom)
	t.Setenv("PATH", "")

	resolved, err := resolveCLIPath()
	if err != nil {
		t.Fatalf("resolveCLIPath error: %v", err)
	}
	if resolved != custom {
		t.Fatalf("resolveCLIPath = %q, want %q", resolved, custom)
	}
}

func TestResolveCLIPathErrorsWithoutDemoItInPath(t *testing.T) {
	t.Setenv("DEMO_IT_PATH", "")
	t.Setenv("PATH", "")

	_, err := resolveCLIPath()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "resolve demo-it via PATH") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKeyMacroPlayback(t *testing.T) {
	interval := 90
	delay := 250
	playback, err := keyMacroPlayback(transcript.Action{
		IntervalMS: &interval,
		Keys: []transcript.KeyMacroKey{
			{Key: "i"},
			{Key: "h"},
			{Key: "return", DelayMS: &delay},
		},
	})
	if err != nil {
		t.Fatalf("keyMacroPlayback error: %v", err)
	}
	if len(playback) != 3 {
		t.Fatalf("expected 3 playback keys, got %d", len(playback))
	}
	if got := playback[0].DelayAfter; got != 90*time.Millisecond {
		t.Fatalf("playback[0].DelayAfter = %v, want %v", got, 90*time.Millisecond)
	}
	if got := playback[2].DelayAfter; got != 250*time.Millisecond {
		t.Fatalf("playback[2].DelayAfter = %v, want %v", got, 250*time.Millisecond)
	}
}

func TestKeyMacroPlaybackRejectsNegativeDelay(t *testing.T) {
	delay := -1
	_, err := keyMacroPlayback(transcript.Action{Keys: []transcript.KeyMacroKey{{Key: "a", DelayMS: &delay}}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "delay_ms") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKeyMacroPlaybackUsesDefaultInterval(t *testing.T) {
	playback, err := keyMacroPlayback(transcript.Action{Keys: []transcript.KeyMacroKey{{Key: "a"}}})
	if err != nil {
		t.Fatalf("keyMacroPlayback error: %v", err)
	}
	if got := playback[0].DelayAfter; got != 80*time.Millisecond {
		t.Fatalf("playback[0].DelayAfter = %v, want %v", got, 80*time.Millisecond)
	}
}

func TestFormatOpenSlideCommand(t *testing.T) {
	got := formatOpenSlideCommand("slides/intro.md")
	want := ":execute 'edit ' . fnameescape('slides/intro.md') | silent! DemoItPresentationEnable"
	if got != want {
		t.Fatalf("formatOpenSlideCommand() = %q, want %q", got, want)
	}
}

func TestFormatOpenSlideCommandEscapesSingleQuote(t *testing.T) {
	got := formatOpenSlideCommand("slides/it's.md")
	want := ":execute 'edit ' . fnameescape('slides/it''s.md') | silent! DemoItPresentationEnable"
	if got != want {
		t.Fatalf("formatOpenSlideCommand() = %q, want %q", got, want)
	}
}

func TestShouldOpenSlide(t *testing.T) {
	if shouldOpenSlide("slides/2-split.md", "slides/2-split.md") {
		t.Fatal("same slide should not trigger open")
	}
	if !shouldOpenSlide("slides/1-intro.md", "slides/2-split.md") {
		t.Fatal("different slides should trigger open")
	}
	if shouldOpenSlide("slides/1-intro.md", "") {
		t.Fatal("empty target slide should not trigger open")
	}
}

func TestIsProtocolCommand(t *testing.T) {
	if !isProtocolCommand("next") {
		t.Fatal("expected next to be protocol command")
	}
	if isProtocolCommand("./examples/demo") {
		t.Fatal("path should not be protocol command")
	}
}

func TestShouldAutoStartOnRunNotFound(t *testing.T) {
	if !shouldAutoStartOnRunNotFound("prev") {
		t.Fatal("prev should auto start")
	}
	if shouldAutoStartOnRunNotFound("status") {
		t.Fatal("status should not auto start")
	}
}

func TestIsLegacyDemoSession(t *testing.T) {
	if !isLegacyDemoSession("demo-demo") {
		t.Fatal("expected demo-demo to be recognized")
	}
	if !isLegacyDemoSession("demo-notes") {
		t.Fatal("expected demo-notes to be recognized")
	}
	if isLegacyDemoSession("main") {
		t.Fatal("main should not be recognized")
	}
}

func TestSelectWorkspacesToKill(t *testing.T) {
	workspaces := []managedWorkspace{
		{Display: "a-demo", SessionNames: []string{"a-demo", "a-notes"}},
		{Display: "b-demo", SessionNames: []string{"b-demo", "b-notes"}},
	}

	all, err := selectWorkspacesToKill(workspaces, nil)
	if err != nil {
		t.Fatalf("select all: %v", err)
	}
	if !reflect.DeepEqual(all, workspaces) {
		t.Fatalf("select all = %#v, want %#v", all, workspaces)
	}

	some, err := selectWorkspacesToKill(workspaces, []string{"2", "1", "2"})
	if err != nil {
		t.Fatalf("select some: %v", err)
	}
	wantSome := []managedWorkspace{workspaces[1], workspaces[0]}
	if !reflect.DeepEqual(some, wantSome) {
		t.Fatalf("select some = %#v, want %#v", some, wantSome)
	}

	if _, err := selectWorkspacesToKill(workspaces, []string{"x"}); err == nil {
		t.Fatal("expected error for non-numeric index")
	}
	if _, err := selectWorkspacesToKill(workspaces, []string{"9"}); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestGroupManagedWorkspacesMergesDemoAndNotes(t *testing.T) {
	sessions := []managedSession{
		{Name: "a-demo", Role: "demo", Workspace: "/tmp/a", LastAttached: 10},
		{Name: "a-notes", Role: "notes", Workspace: "/tmp/a", LastAttached: 9},
	}

	workspaces := groupManagedWorkspaces(sessions)
	if len(workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(workspaces))
	}
	if workspaces[0].Display != "a-demo" {
		t.Fatalf("unexpected workspace display: %s", workspaces[0].Display)
	}
}

func TestSelectWorkspaceSessionsChoosesLatestWorkspace(t *testing.T) {
	sessions := []managedSession{
		{Name: "a-demo", Role: "demo", Workspace: "/tmp/a", Step: 1, LastAttached: 10},
		{Name: "a-notes", Role: "notes", Workspace: "/tmp/a", Step: 1, LastAttached: 10},
		{Name: "b-demo", Role: "demo", Workspace: "/tmp/b", Step: 2, LastAttached: 20},
		{Name: "b-notes", Role: "notes", Workspace: "/tmp/b", Step: 2, LastAttached: 20},
	}

	demo, notes, ok := selectWorkspaceSessions(sessions)
	if !ok {
		t.Fatal("expected workspace sessions")
	}
	if demo.Name != "b-demo" {
		t.Fatalf("unexpected demo session: %s", demo.Name)
	}
	if notes == nil || notes.Name != "b-notes" {
		t.Fatalf("unexpected notes session: %#v", notes)
	}
}

func TestLatestWorkspaceSessionByRole(t *testing.T) {
	workspaces := []managedWorkspace{
		{Display: "a-demo", DemoSession: "a-demo", NotesSession: "a-notes"},
		{Display: "b-demo", DemoSession: "b-demo", NotesSession: "b-notes"},
	}

	if got := latestWorkspaceSessionByRole(workspaces, "demo"); got != "a-demo" {
		t.Fatalf("latestWorkspaceSessionByRole(demo) = %q, want a-demo", got)
	}
	if got := latestWorkspaceSessionByRole(workspaces, "notes"); got != "a-notes" {
		t.Fatalf("latestWorkspaceSessionByRole(notes) = %q, want a-notes", got)
	}
	if got := latestWorkspaceSessionByRole(workspaces, "unknown"); got != "" {
		t.Fatalf("latestWorkspaceSessionByRole(unknown) = %q, want empty", got)
	}
}

func TestLatestWorkspaceSessionByRoleSkipsEmptyEntries(t *testing.T) {
	workspaces := []managedWorkspace{
		{Display: "a-demo", DemoSession: "a-demo"},
		{Display: "b-demo", NotesSession: "b-notes"},
	}

	if got := latestWorkspaceSessionByRole(workspaces, "notes"); got != "b-notes" {
		t.Fatalf("latestWorkspaceSessionByRole(notes) = %q, want b-notes", got)
	}
}

func TestSelectPaneToKeep(t *testing.T) {
	panes := []paneState{{ID: "%9", Index: 2}, {ID: "%1", Index: 0}, {ID: "%2", Index: 1}}
	if got := selectPaneToKeep(panes); got != "%1" {
		t.Fatalf("selectPaneToKeep = %q, want %%1", got)
	}
}

func TestSelectEditorPanePrefersActiveEditor(t *testing.T) {
	panes := []paneState{
		{ID: "%1", Command: "nvim", Active: false},
		{ID: "%2", Command: "zsh", Active: true},
		{ID: "%3", Command: "nvim", Active: true},
	}

	got, ok := selectEditorPane(panes)
	if !ok {
		t.Fatal("expected editor pane")
	}
	if got != "%3" {
		t.Fatalf("selectEditorPane = %q, want %%3", got)
	}
}

func TestSelectEditorPaneFallsBackToAnyEditor(t *testing.T) {
	panes := []paneState{
		{ID: "%1", Command: "zsh", Active: true},
		{ID: "%2", Command: "nvim", Active: false},
	}

	got, ok := selectEditorPane(panes)
	if !ok {
		t.Fatal("expected editor pane")
	}
	if got != "%2" {
		t.Fatalf("selectEditorPane = %q, want %%2", got)
	}
}

func TestSelectEditorPaneReturnsFalseWithoutEditor(t *testing.T) {
	panes := []paneState{{ID: "%1", Command: "zsh", Active: true}}
	if _, ok := selectEditorPane(panes); ok {
		t.Fatal("expected no editor pane")
	}
}

func TestSelectPaneForEditorStartPrefersActivePane(t *testing.T) {
	panes := []paneState{{ID: "%1", Index: 0, Active: false}, {ID: "%2", Index: 1, Active: true}}
	got, ok := selectPaneForEditorStart(panes)
	if !ok {
		t.Fatal("expected pane")
	}
	if got != "%2" {
		t.Fatalf("selectPaneForEditorStart = %q, want %%2", got)
	}
}

func TestSelectPaneForEditorStartFallsBackToLowestIndex(t *testing.T) {
	panes := []paneState{{ID: "%9", Index: 2}, {ID: "%1", Index: 0}, {ID: "%2", Index: 1}}
	got, ok := selectPaneForEditorStart(panes)
	if !ok {
		t.Fatal("expected pane")
	}
	if got != "%1" {
		t.Fatalf("selectPaneForEditorStart = %q, want %%1", got)
	}
}

func TestSelectPaneBySelector(t *testing.T) {
	panes := []paneState{{ID: "%1", Index: 0, Active: false}, {ID: "%2", Index: 1, Active: true}, {ID: "%3", Index: 2, Active: false}}

	if got, ok := selectPaneBySelector(panes, "active"); !ok || got != "%2" {
		t.Fatalf("selectPaneBySelector(active) = (%q,%v), want (%%2,true)", got, ok)
	}
	if got, ok := selectPaneBySelector(panes, "last"); !ok || got != "%3" {
		t.Fatalf("selectPaneBySelector(last) = (%q,%v), want (%%3,true)", got, ok)
	}
	if got, ok := selectPaneBySelector(panes, "left"); !ok || got != "%1" {
		t.Fatalf("selectPaneBySelector(left) = (%q,%v), want (%%1,true)", got, ok)
	}
	if got, ok := selectPaneBySelector(panes, "1"); !ok || got != "%2" {
		t.Fatalf("selectPaneBySelector(1) = (%q,%v), want (%%2,true)", got, ok)
	}
	if got, ok := selectPaneBySelector(panes, "%3"); !ok || got != "%3" {
		t.Fatalf("selectPaneBySelector(%%3) = (%q,%v), want (%%3,true)", got, ok)
	}
	if _, ok := selectPaneBySelector(panes, "missing"); ok {
		t.Fatal("expected selector miss")
	}
}

func TestPaneIDsExcludingKeep(t *testing.T) {
	panes := []paneState{{ID: "%1"}, {ID: "%2"}, {ID: "%3"}}
	got := paneIDsExcludingKeep(panes, "%1")
	want := []string{"%2", "%3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paneIDsExcludingKeep = %#v, want %#v", got, want)
	}
}

func TestPaneCommandByID(t *testing.T) {
	panes := []paneState{{ID: "%1", Command: "nvim"}, {ID: "%2", Command: "zsh"}}
	if got := paneCommandByID(panes, "%2"); got != "zsh" {
		t.Fatalf("paneCommandByID = %q, want zsh", got)
	}
	if got := paneCommandByID(panes, "%9"); got != "" {
		t.Fatalf("paneCommandByID missing = %q, want empty", got)
	}
}

func TestShouldResetPaneAfterClear(t *testing.T) {
	if shouldResetPaneAfterClear("nvim") {
		t.Fatal("nvim pane should not be reset")
	}
	if !shouldResetPaneAfterClear("zsh") {
		t.Fatal("shell pane should be reset")
	}
}

func TestNormalizeStepIndex(t *testing.T) {
	if got := normalizeStepIndex(-1, 3); got != 0 {
		t.Fatalf("normalizeStepIndex(-1,3) = %d, want 0", got)
	}
	if got := normalizeStepIndex(9, 3); got != 2 {
		t.Fatalf("normalizeStepIndex(9,3) = %d, want 2", got)
	}
	if got := normalizeStepIndex(1, 3); got != 1 {
		t.Fatalf("normalizeStepIndex(1,3) = %d, want 1", got)
	}
}

func TestMergeWorkspaceTransition(t *testing.T) {
	state := map[string]any{"current_slide": 1}
	merged, err := mergeWorkspaceTransition(state, workspaceStepTransition{
		Available:       true,
		StepIndex:       2,
		TotalSteps:      5,
		StepTitle:       "Open split layout slide",
		ActionsExecuted: true,
	})
	if err != nil {
		t.Fatalf("mergeWorkspaceTransition: %v", err)
	}
	if got, ok := merged["workspace_step"].(int); !ok || got != 2 {
		t.Fatalf("workspace_step = %#v, want 2", merged["workspace_step"])
	}
	if merged["workspace_step_title"] != "Open split layout slide" {
		t.Fatalf("workspace_step_title = %#v", merged["workspace_step_title"])
	}
}

func TestResolveStepTransition(t *testing.T) {
	next, execute := resolveStepTransition(0, 3, "next")
	if next != 1 || !execute {
		t.Fatalf("next transition = (%d,%v), want (1,true)", next, execute)
	}

	prev, execute := resolveStepTransition(1, 3, "prev")
	if prev != 0 || execute {
		t.Fatalf("prev transition = (%d,%v), want (0,false)", prev, execute)
	}

	stays, execute := resolveStepTransition(0, 3, "prev")
	if stays != 0 || execute {
		t.Fatalf("prev-at-start transition = (%d,%v), want (0,false)", stays, execute)
	}
}

func TestExecuteBootstrapStepRequiresTranscriptWhenRequested(t *testing.T) {
	workspace := t.TempDir()
	_, _, err := executeBootstrapStep(workspace, "demo-session", true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing transcript") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteBootstrapStepAllowsMissingTranscriptWhenOptional(t *testing.T) {
	workspace := t.TempDir()
	steps, step, err := executeBootstrapStep(workspace, "demo-session", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if steps != nil {
		t.Fatalf("expected nil steps, got %#v", steps)
	}
	if step != -1 {
		t.Fatalf("expected step -1, got %d", step)
	}
}

func TestRenderSpeakerNotes(t *testing.T) {
	steps := []transcript.Step{
		{Title: "Step one", SpeakerNotes: "first note"},
		{Title: "Step two", SpeakerNotes: "second note"},
	}

	rendered := renderSpeakerNotes(steps, 0)
	want := "first note\n\n---\nNext: Step two\n"
	if rendered != want {
		t.Fatalf("unexpected notes output: %q", rendered)
	}

	rendered = renderSpeakerNotes(steps, 9)
	if rendered != "second note\n" {
		t.Fatalf("unexpected clamped notes output: %q", rendered)
	}
}
