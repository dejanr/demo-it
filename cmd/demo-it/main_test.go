package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dejanr/demo-it/internal/protocol"
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

func TestAutoNextArgsForStepUsesResolvedCLIPath(t *testing.T) {
	temp := t.TempDir()
	custom := filepath.Join(temp, "demo-it-local")
	if err := os.WriteFile(custom, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write custom demo-it: %v", err)
	}

	auto := 500
	steps := []transcript.Step{
		{Title: "one", AutoSlideInMS: &auto, Actions: []transcript.Action{{Kind: "key", Key: "return"}}},
		{Title: "two", Actions: []transcript.Action{{Kind: "key", Key: "return"}}},
	}

	t.Setenv("DEMO_IT_PATH", custom)
	t.Setenv("PATH", "")

	args, err := autoNextArgsForStep(steps, 0, "/tmp/demo.sock")
	if err != nil {
		t.Fatalf("autoNextArgsForStep error: %v", err)
	}
	if !args.Enabled {
		t.Fatal("expected auto-next to be enabled")
	}
	if args.CLIPath != custom {
		t.Fatalf("CLIPath = %q, want %q", args.CLIPath, custom)
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
	if !isProtocolCommand("run-status") {
		t.Fatal("expected run-status to be protocol command")
	}
	if !isProtocolCommand("list") {
		t.Fatal("expected list to be treated as reserved command")
	}
	if isProtocolCommand("status") {
		t.Fatal("status should be session command")
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

	if _, err := selectWorkspacesToKill(workspaces, []string{"1"}); err == nil {
		t.Fatal("expected error when kill is called with indexes")
	}
}

func TestManagedWorkspaceSessionNamesDeduplicates(t *testing.T) {
	workspaces := []managedWorkspace{
		{Display: "a-demo", SessionNames: []string{"a-demo", "a-notes", ""}},
		{Display: "b-demo", SessionNames: []string{"b-demo", "a-notes", "b-notes"}},
	}

	got := managedWorkspaceSessionNames(workspaces)
	want := []string{"a-demo", "a-notes", "b-demo", "b-notes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("managedWorkspaceSessionNames = %#v, want %#v", got, want)
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

func TestLatestWorkspaceByRole(t *testing.T) {
	workspaces := []managedWorkspace{
		{Display: "a-demo", DemoSession: "a-demo", NotesSession: "a-notes", Workspace: "/tmp/a"},
		{Display: "b-demo", DemoSession: "b-demo", NotesSession: "b-notes", Workspace: "/tmp/b"},
	}

	got, ok := latestWorkspaceByRole(workspaces, "demo")
	if !ok {
		t.Fatal("expected latest demo workspace")
	}
	if got.Workspace != "/tmp/a" {
		t.Fatalf("latestWorkspaceByRole(demo).Workspace = %q, want /tmp/a", got.Workspace)
	}

	if _, ok := latestWorkspaceByRole(workspaces, "unknown"); ok {
		t.Fatal("expected unknown role to be empty")
	}
}

func TestParseActivePaneIDOutput(t *testing.T) {
	paneID, err := parseActivePaneIDOutput("%7\t0\n%9\t1\n")
	if err != nil {
		t.Fatalf("parseActivePaneIDOutput error: %v", err)
	}
	if paneID != "%9" {
		t.Fatalf("parseActivePaneIDOutput = %q, want %%9", paneID)
	}
}

func TestParseActivePaneIDOutputFallsBackToFirstPane(t *testing.T) {
	paneID, err := parseActivePaneIDOutput("%3\t0\n%8\t0\n")
	if err != nil {
		t.Fatalf("parseActivePaneIDOutput error: %v", err)
	}
	if paneID != "%3" {
		t.Fatalf("parseActivePaneIDOutput = %q, want %%3", paneID)
	}
}

func TestParseActivePaneIDOutputErrorsWhenMissing(t *testing.T) {
	_, err := parseActivePaneIDOutput("\n")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTraceLogPath(t *testing.T) {
	now := time.Date(2026, time.March, 2, 13, 20, 10, 0, time.UTC)
	got := traceLogPath("/tmp/workspace", "next", "%12", now)
	want := filepath.Join("/tmp/workspace", ".demo-it", "traces", "20260302-132010-next-pane-12.log")
	if got != want {
		t.Fatalf("traceLogPath = %q, want %q", got, want)
	}
}

func TestTraceTextSnapshotPath(t *testing.T) {
	got := traceTextSnapshotPath("/tmp/workspace/.demo-it/traces/trace.log")
	want := "/tmp/workspace/.demo-it/traces/trace.txt"
	if got != want {
		t.Fatalf("traceTextSnapshotPath = %q, want %q", got, want)
	}
}

func TestNormalizeTraceForTextSnapshotGolden(t *testing.T) {
	raw := "# demo-it trace-next\n" +
		"# session: demo-demo\n" +
		"# pane: %12\n" +
		"# timestamp: 2026-03-02T14:30:00Z\n\n" +
		"\x1b]0;demo-it\x07\x1b[?25l$ printf 'hello\\n'\r\nhello\r\n$ \x1b[0m\x1bP+q4D73\x1b\\"

	got := normalizeTraceForTextSnapshot(raw)
	goldenPath := filepath.Join("testdata", "trace-normalized.golden")
	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	want := string(wantBytes)
	if got != want {
		t.Fatalf("normalizeTraceForTextSnapshot mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteTraceTextSnapshot(t *testing.T) {
	dir := t.TempDir()
	rawPath := filepath.Join(dir, "trace.log")
	raw := "# demo-it trace-next\n# session: x\n# pane: %1\n# timestamp: now\n\n$ echo hi\r\nhi\r\n"
	if err := os.WriteFile(rawPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write raw trace: %v", err)
	}

	textPath, err := writeTraceTextSnapshot(rawPath)
	if err != nil {
		t.Fatalf("writeTraceTextSnapshot: %v", err)
	}
	if textPath != filepath.Join(dir, "trace.txt") {
		t.Fatalf("textPath = %q, want %q", textPath, filepath.Join(dir, "trace.txt"))
	}
	bytes, err := os.ReadFile(textPath)
	if err != nil {
		t.Fatalf("read text trace: %v", err)
	}
	if string(bytes) != "$ echo hi\nhi\n" {
		t.Fatalf("unexpected text snapshot: %q", string(bytes))
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

func TestStepAutoSlideDelayMS(t *testing.T) {
	auto := 1250
	steps := []transcript.Step{
		{Title: "one", Actions: []transcript.Action{{Kind: "key", Key: "return"}}},
		{Title: "two", AutoSlideInMS: &auto, Actions: []transcript.Action{{Kind: "key", Key: "return"}}},
		{Title: "three", Actions: []transcript.Action{{Kind: "key", Key: "return"}}},
	}

	if got, ok := stepAutoSlideDelayMS(steps, 1); !ok || got != 1250 {
		t.Fatalf("stepAutoSlideDelayMS(step=1) = (%d,%v), want (1250,true)", got, ok)
	}
	if _, ok := stepAutoSlideDelayMS(steps, 2); ok {
		t.Fatal("last step should not auto-advance")
	}
	if _, ok := stepAutoSlideDelayMS(steps, -1); ok {
		t.Fatal("invalid index should not auto-advance")
	}
}

func TestAutoNextArgsForStepDisablesWhenNoTimer(t *testing.T) {
	steps := []transcript.Step{
		{Title: "one", Actions: []transcript.Action{{Kind: "key", Key: "return"}}},
		{Title: "two", Actions: []transcript.Action{{Kind: "key", Key: "return"}}},
	}

	args, err := autoNextArgsForStep(steps, 0, "/tmp/demo.sock")
	if err != nil {
		t.Fatalf("autoNextArgsForStep error: %v", err)
	}
	if args.Enabled {
		t.Fatalf("args = %#v, want disabled", args)
	}
	if args.DelayMS != 0 || args.CLIPath != "" || args.SocketPath != "" || args.DebugLogPath != "" || len(args.Env) != 0 {
		t.Fatalf("args = %#v, want zero-value disabled args", args)
	}
}

func TestStateSupportsCapability(t *testing.T) {
	state := map[string]interface{}{
		"capabilities": []interface{}{protocol.CapabilitySetAutoNext, "other"},
	}
	if !stateSupportsCapability(state, protocol.CapabilitySetAutoNext) {
		t.Fatal("expected capability to be detected")
	}
	if stateSupportsCapability(state, "missing") {
		t.Fatal("expected missing capability to be false")
	}
}

func TestStateSupportsCapabilityHandlesMissingShape(t *testing.T) {
	if stateSupportsCapability(map[string]interface{}{}, protocol.CapabilitySetAutoNext) {
		t.Fatal("expected false for missing capabilities field")
	}
	if stateSupportsCapability("invalid", protocol.CapabilitySetAutoNext) {
		t.Fatal("expected false for non-map state")
	}
}

func TestNotesPaneCommand(t *testing.T) {
	command := notesPaneCommand("hello notes")
	if !strings.Contains(command, "base64 -d") {
		t.Fatalf("expected base64 decode pipeline, got %q", command)
	}
	if !strings.Contains(command, "[demo-it-notes]") {
		t.Fatalf("expected notes buffer name, got %q", command)
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

func TestResolveRecordWorkspacePath(t *testing.T) {
	workspace := t.TempDir()
	resolved, err := resolveRecordWorkspacePath([]string{workspace})
	if err != nil {
		t.Fatalf("resolveRecordWorkspacePath error: %v", err)
	}
	if resolved != workspace {
		t.Fatalf("resolveRecordWorkspacePath = %q, want %q", resolved, workspace)
	}
}

func TestResolveRecordWorkspacePathRejectsMultiplePaths(t *testing.T) {
	_, err := resolveRecordWorkspacePath([]string{"a", "b"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "at most one workspace path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordedActionsCollapsesMultipleKillPaneEvents(t *testing.T) {
	events := []recordEvent{
		{Kind: "split-pane"},
		{Kind: "kill-pane"},
		{Kind: "kill-pane"},
	}

	actions := recordedActions(events)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].Kind != "split-pane" {
		t.Fatalf("actions[0].Kind = %q, want split-pane", actions[0].Kind)
	}
	if actions[1].Kind != "killall-pane" {
		t.Fatalf("actions[1].Kind = %q, want killall-pane", actions[1].Kind)
	}
}

func TestRenderRecordedBlockAddsFallbackActionWhenEmpty(t *testing.T) {
	block, err := renderRecordedBlock("", nil)
	if err != nil {
		t.Fatalf("renderRecordedBlock error: %v", err)
	}
	if !strings.Contains(block, "```demo-it") {
		t.Fatalf("expected fenced block, got %q", block)
	}
	if !strings.Contains(block, "title: Recorded interaction") {
		t.Fatalf("expected default title, got %q", block)
	}
	if !strings.Contains(block, "kind: insert-text") {
		t.Fatalf("expected fallback action, got %q", block)
	}
}

func TestIsRecordEventKind(t *testing.T) {
	if !isRecordEventKind("split-pane") {
		t.Fatal("expected split-pane to be supported")
	}
	if isRecordEventKind("insert-text") {
		t.Fatal("insert-text should not be a raw record event")
	}
}

func TestRecordSplitKindFromHookArguments(t *testing.T) {
	if got := recordSplitKindFromHookArguments("-h -t demo-it-record"); got != "split-pane" {
		t.Fatalf("recordSplitKindFromHookArguments(-h) = %q, want split-pane", got)
	}
	if got := recordSplitKindFromHookArguments("-v -t demo-it-record"); got != "split-pane-vertical" {
		t.Fatalf("recordSplitKindFromHookArguments(-v) = %q, want split-pane-vertical", got)
	}
	if got := recordSplitKindFromHookArguments(""); got != "split-pane" {
		t.Fatalf("recordSplitKindFromHookArguments(empty) = %q, want split-pane", got)
	}
}

func TestRecordPaneLogLabelAndSelector(t *testing.T) {
	label := recordPaneLogLabel("%12", "1")
	if label != "pane-1-12" {
		t.Fatalf("recordPaneLogLabel = %q, want pane-1-12", label)
	}
	if selector := recordPaneSelectorFromLogName(label + ".timing.log"); selector != "1" {
		t.Fatalf("recordPaneSelectorFromLogName = %q, want 1", selector)
	}
	if selector := recordPaneSelectorFromLogName("pane-0-1.timing.log"); selector != "" {
		t.Fatalf("recordPaneSelectorFromLogName pane-0 should be empty selector, got %q", selector)
	}
}

func TestExtractRecordInputPayload(t *testing.T) {
	raw := []byte("Script started on now\nls\nexit\n\nScript done on now\n")
	payload, err := extractRecordInputPayload(raw, len("ls\nexit\n"))
	if err != nil {
		t.Fatalf("extractRecordInputPayload error: %v", err)
	}
	if string(payload) != "ls\nexit\n" {
		t.Fatalf("extractRecordInputPayload = %q, want %q", string(payload), "ls\\nexit\\n")
	}
}

func TestParseRecordInputTimingAccumulatesAllRecordDeltas(t *testing.T) {
	timingPath := filepath.Join(t.TempDir(), "timing.log")
	content := "H 0.000000 START_TIME 2026-03-03 14:14:34+01:00\n" +
		"O 0.300000 10\n" +
		"I 0.200000 1\n"
	if err := os.WriteFile(timingPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write timing file: %v", err)
	}
	chunks, total, err := parseRecordInputTiming(timingPath, nil)
	if err != nil {
		t.Fatalf("parseRecordInputTiming error: %v", err)
	}
	if total != 1 || len(chunks) != 1 {
		t.Fatalf("unexpected chunks/total: %#v total=%d", chunks, total)
	}
	start, err := time.Parse("2006-01-02 15:04:05-07:00", "2026-03-03 14:14:34+01:00")
	if err != nil {
		t.Fatalf("parse start time: %v", err)
	}
	want := start.Add(500 * time.Millisecond)
	if !chunks[0].Timestamp.Equal(want) {
		t.Fatalf("chunk timestamp = %s, want %s", chunks[0].Timestamp, want)
	}
}

func TestParseRecordInputTimingUsesStartOverride(t *testing.T) {
	timingPath := filepath.Join(t.TempDir(), "timing.log")
	content := "H 0.000000 START_TIME 2026-03-03 14:14:34+01:00\n" +
		"I 0.200000 1\n"
	if err := os.WriteFile(timingPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write timing file: %v", err)
	}
	override := time.Date(2026, time.March, 3, 13, 14, 34, 750000000, time.UTC)
	chunks, total, err := parseRecordInputTiming(timingPath, &override)
	if err != nil {
		t.Fatalf("parseRecordInputTiming error: %v", err)
	}
	if total != 1 || len(chunks) != 1 {
		t.Fatalf("unexpected chunks/total: %#v total=%d", chunks, total)
	}
	want := override.Add(200 * time.Millisecond)
	if !chunks[0].Timestamp.Equal(want) {
		t.Fatalf("chunk timestamp = %s, want %s", chunks[0].Timestamp, want)
	}
}

func TestRecordActionsFromInputChunk(t *testing.T) {
	timestamp := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	actions := recordActionsFromInputChunk([]byte("ls\n\t\x1b[A\x7f\x04"), timestamp, "1")
	if len(actions) != 2 {
		t.Fatalf("expected 2 key-macro actions, got %d", len(actions))
	}
	first := actions[0].Action
	if first.Kind != "key-macro" || first.Pane != "1" {
		t.Fatalf("unexpected first action: %#v", first)
	}
	firstWant := []string{"l", "s", "Enter"}
	if len(first.Keys) != len(firstWant) {
		t.Fatalf("first key count = %d, want %d", len(first.Keys), len(firstWant))
	}
	for i, expected := range firstWant {
		if first.Keys[i].Key != expected {
			t.Fatalf("first.Keys[%d] = %q, want %q", i, first.Keys[i].Key, expected)
		}
	}

	second := actions[1].Action
	if second.Kind != "key-macro" || second.Pane != "1" {
		t.Fatalf("unexpected second action: %#v", second)
	}
	secondWant := []string{"Tab", "Up", "BSpace", "C-d"}
	if len(second.Keys) != len(secondWant) {
		t.Fatalf("second key count = %d, want %d", len(second.Keys), len(secondWant))
	}
	for i, expected := range secondWant {
		if second.Keys[i].Key != expected {
			t.Fatalf("second.Keys[%d] = %q, want %q", i, second.Keys[i].Key, expected)
		}
	}
}

func TestRecordTimedActionsFromKeyEventsSplitsOnEnterAndTracksDelay(t *testing.T) {
	base := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	events := []recordedKeyEvent{
		{Timestamp: base, Key: "C-r"},
		{Timestamp: base.Add(40 * time.Millisecond), Key: "e"},
		{Timestamp: base.Add(80 * time.Millisecond), Key: "c"},
		{Timestamp: base.Add(120 * time.Millisecond), Key: "h"},
		{Timestamp: base.Add(160 * time.Millisecond), Key: "o"},
		{Timestamp: base.Add(220 * time.Millisecond), Key: "Enter"},
		{Timestamp: base.Add(400 * time.Millisecond), Key: "C-d"},
	}
	actions := recordTimedActionsFromKeyEvents(events, "")
	if len(actions) != 2 {
		t.Fatalf("expected 2 key-macro actions, got %d", len(actions))
	}
	first := actions[0].Action
	if first.Kind != "key-macro" {
		t.Fatalf("first action kind = %q, want key-macro", first.Kind)
	}
	wantFirst := []string{"C-r", "e", "c", "h", "o", "Enter"}
	if len(first.Keys) != len(wantFirst) {
		t.Fatalf("first key count = %d, want %d", len(first.Keys), len(wantFirst))
	}
	for i, expected := range wantFirst {
		if first.Keys[i].Key != expected {
			t.Fatalf("first.Keys[%d] = %q, want %q", i, first.Keys[i].Key, expected)
		}
	}
	if first.DelayMS == nil || *first.DelayMS != 500 {
		t.Fatalf("first.DelayMS = %#v, want 500", first.DelayMS)
	}
	if first.Keys[0].DelayMS == nil || *first.Keys[0].DelayMS != 40 {
		t.Fatalf("first.Keys[0].DelayMS = %#v, want 40", first.Keys[0].DelayMS)
	}
	second := actions[1].Action
	if second.Kind != "key-macro" || len(second.Keys) != 1 || second.Keys[0].Key != "C-d" {
		t.Fatalf("unexpected second action: %#v", second)
	}
	if second.DelayMS == nil || *second.DelayMS != 500 {
		t.Fatalf("second.DelayMS = %#v, want 500", second.DelayMS)
	}
}
