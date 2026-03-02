package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dejanr/demo-it/internal/client"
	"github.com/dejanr/demo-it/internal/protocol"
	"github.com/dejanr/demo-it/internal/runctx"
	"github.com/dejanr/demo-it/internal/transcript"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	repoRoot, err := runctx.RepoRoot()
	if err != nil {
		return err
	}

	defaultRunID := runctx.DefaultRunID(repoRoot)
	defaultSocketPath := runctx.DefaultSocketPath(repoRoot)

	runID := flag.String("run-id", defaultRunID, "run identifier")
	socketPath := flag.String("socket", defaultSocketPath, "daemon unix socket path")
	flag.Parse()

	if flag.NArg() == 0 {
		return usageError()
	}

	target := flag.Arg(0)
	switch target {
	case "list":
		return listSessionsCommand()
	case "kill":
		return killSessionsCommand(flag.Args()[1:])
	case "notes":
		return notesCommand()
	case "show":
		return showCommand()
	case "start":
		workspacePath := ""
		switch flag.NArg() {
		case 1:
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve current working directory: %w", err)
			}
			workspacePath = cwd
		case 2:
			workspacePath = flag.Arg(1)
		default:
			return fmt.Errorf("start accepts at most one workspace path\n%s", usage)
		}
		return bootstrapWorkspace(workspacePath, *runID, *socketPath, true)
	}

	if !isProtocolCommand(target) {
		return bootstrapWorkspace(target, *runID, *socketPath, false)
	}

	cmd, args, err := parseSubcommand(target, flag.Args()[1:])
	if err != nil {
		return err
	}

	req := protocol.Request{
		ID:      fmt.Sprintf("cli-%d", time.Now().UnixNano()),
		Command: cmd,
		RunID:   *runID,
		Args:    args,
	}

	c := client.SocketClient{SocketPath: *socketPath}
	resp, err := sendWithAutoStart(c, req, target, *runID, *socketPath)
	if err != nil {
		return err
	}

	if !resp.OK {
		if resp.Error == nil {
			return errors.New("daemon returned unknown error")
		}
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	workspaceTransition := workspaceStepTransition{}
	if target == "next" || target == "prev" || target == "rerun" {
		workspaceTransition, err = advanceWorkspaceStepForCommand(target)
		if err != nil {
			return err
		}
	}

	responseState := resp.State
	if workspaceTransition.Available {
		responseState, err = mergeWorkspaceTransition(resp.State, workspaceTransition)
		if err != nil {
			return err
		}
	}

	bytes, err := json.MarshalIndent(responseState, "", "  ")
	if err != nil {
		return fmt.Errorf("format response: %w", err)
	}
	fmt.Println(string(bytes))

	return nil
}

func sendWithAutoStart(c client.SocketClient, req protocol.Request, command string, runID string, socketPath string) (protocol.Response, error) {
	resp, err := c.Send(req)
	if err != nil {
		return protocol.Response{}, err
	}

	if resp.OK || resp.Error == nil {
		return resp, nil
	}
	if !shouldAutoStartOnRunNotFound(command) || resp.Error.Code != "run_not_found" {
		return resp, nil
	}

	if err := ensureRunStarted(runID, socketPath); err != nil {
		return protocol.Response{}, err
	}

	resp, err = c.Send(req)
	if err != nil {
		return protocol.Response{}, err
	}
	return resp, nil
}

func shouldAutoStartOnRunNotFound(command string) bool {
	switch command {
	case "next", "prev", "rerun", "jump", "focus", "reload":
		return true
	default:
		return false
	}
}

func bootstrapWorkspace(rawPath string, runID string, socketPath string, requireTranscript bool) error {
	workspacePath, err := filepath.Abs(rawPath)
	if err != nil {
		return fmt.Errorf("resolve workspace path: %w", err)
	}

	stat, err := os.Stat(workspacePath)
	if err != nil {
		return fmt.Errorf("workspace path: %w", err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("workspace path is not a directory: %s", workspacePath)
	}

	demoSession := runctx.DemoSessionName(workspacePath)
	notesSession := runctx.NotesSessionName(workspacePath)

	if err := resetSession(demoSession); err != nil {
		return err
	}
	if err := resetSession(notesSession); err != nil {
		return err
	}

	if err := createSession(demoSession, workspacePath); err != nil {
		return err
	}
	if err := createSession(notesSession, workspacePath); err != nil {
		return err
	}

	steps, currentStep, err := executeBootstrapStep(workspacePath, demoSession, requireTranscript)
	if err != nil {
		return err
	}

	if err := setSessionMetadata(demoSession, workspacePath, "demo", currentStep); err != nil {
		return err
	}
	if err := setSessionMetadata(notesSession, workspacePath, "notes", currentStep); err != nil {
		return err
	}

	if err := refreshNotesSession(notesSession, workspacePath, steps, currentStep); err != nil {
		return err
	}

	if err := ensureRunStarted(runID, socketPath); err != nil {
		return err
	}
	if err := ensureDemoNavigationBindings(); err != nil {
		return err
	}

	if os.Getenv("TMUX") != "" {
		return runTmuxWithStdio("switch-client", "-t", demoSession)
	}
	return runTmuxWithStdio("attach-session", "-t", demoSession)
}

func resetSession(name string) error {
	exists, err := tmuxSessionExists(name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if err := runTmux("kill-session", "-t", name); err != nil {
		return fmt.Errorf("reset tmux session %q: %w", name, err)
	}
	return nil
}

func createSession(name string, cwd string) error {
	if err := runTmux("new-session", "-d", "-s", name, "-c", cwd); err != nil {
		return fmt.Errorf("create tmux session %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it", "1"); err != nil {
		return fmt.Errorf("mark tmux session %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "status", "off"); err != nil {
		return fmt.Errorf("hide tmux status for %q: %w", name, err)
	}
	return nil
}

func executeBootstrapStep(workspacePath string, demoSession string, requireTranscript bool) ([]transcript.Step, int, error) {
	stepsPath := filepath.Join(workspacePath, "demo-it.md")
	steps, err := transcript.ParseStepsFile(stepsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if requireTranscript {
				return nil, -1, fmt.Errorf("missing transcript: %s", stepsPath)
			}
			return nil, -1, nil
		}
		return nil, -1, fmt.Errorf("parse %s: %w", stepsPath, err)
	}
	if len(steps) == 0 {
		return steps, -1, nil
	}
	if err := runActions(demoSession, steps[0].Actions); err != nil {
		return nil, -1, err
	}
	return steps, 0, nil
}

func runActions(targetSession string, actions []transcript.Action) error {
	for _, action := range actions {
		switch action.Kind {
		case "insert-text":
			if err := runTmux("send-keys", "-t", targetSession, action.Text); err != nil {
				return fmt.Errorf("insert-text action: %w", err)
			}
		case "key":
			if err := runTmux("send-keys", "-t", targetSession, tmuxKey(action.Key)); err != nil {
				return fmt.Errorf("key action: %w", err)
			}
		case "split-pane":
			if err := splitPaneAction(targetSession, action.Direction); err != nil {
				return err
			}
		case "split-pane-vertical":
			if err := splitPaneAction(targetSession, "down"); err != nil {
				return err
			}
		case "clear-panes":
			if err := clearPanesAction(targetSession); err != nil {
				return err
			}
		case "open-slide":
			if err := openSlideAction(targetSession, action.Path); err != nil {
				return err
			}
		case "key-macro":
			if err := keyMacroAction(targetSession, action); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported action kind: %s", action.Kind)
		}
	}
	return nil
}

func tmuxKey(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "enter", "return":
		return "Enter"
	default:
		return key
	}
}

type keyMacroPlaybackStep struct {
	Key        string
	DelayAfter time.Duration
}

func keyMacroAction(targetSession string, action transcript.Action) error {
	playback, err := keyMacroPlayback(action)
	if err != nil {
		return fmt.Errorf("key-macro action: %w", err)
	}

	for i, step := range playback {
		if err := runTmux("send-keys", "-t", targetSession, tmuxKey(step.Key)); err != nil {
			return fmt.Errorf("key-macro action (step %d): %w", i, err)
		}
		if step.DelayAfter > 0 && i+1 < len(playback) {
			time.Sleep(step.DelayAfter)
		}
	}
	return nil
}

func keyMacroPlayback(action transcript.Action) ([]keyMacroPlaybackStep, error) {
	if len(action.Keys) == 0 {
		return nil, errors.New("requires at least one key")
	}

	defaultInterval := 80
	if action.IntervalMS != nil {
		if *action.IntervalMS < 0 {
			return nil, errors.New("interval_ms must be >= 0")
		}
		defaultInterval = *action.IntervalMS
	}

	playback := make([]keyMacroPlaybackStep, 0, len(action.Keys))
	for i, macroKey := range action.Keys {
		key := strings.TrimSpace(macroKey.Key)
		if key == "" {
			return nil, fmt.Errorf("key at index %d is empty", i)
		}

		delayMS := defaultInterval
		if macroKey.DelayMS != nil {
			if *macroKey.DelayMS < 0 {
				return nil, fmt.Errorf("delay_ms at index %d must be >= 0", i)
			}
			delayMS = *macroKey.DelayMS
		}
		playback = append(playback, keyMacroPlaybackStep{
			Key:        key,
			DelayAfter: time.Duration(delayMS) * time.Millisecond,
		})
	}

	return playback, nil
}

func splitPaneAction(targetSession string, direction string) error {
	flag := "-h"
	if strings.TrimSpace(direction) == "down" {
		flag = "-v"
	}
	if err := runTmux("split-window", "-d", flag, "-t", targetSession); err != nil {
		return fmt.Errorf("split-pane action: %w", err)
	}
	return nil
}

func clearPanesAction(targetSession string) error {
	panes, err := listSessionPanes(targetSession)
	if err != nil {
		return err
	}
	if len(panes) == 0 {
		return nil
	}

	keepPane := selectPaneToKeep(panes)
	keepCommand := paneCommandByID(panes, keepPane)
	if len(panes) > 1 {
		if err := closeExtraPanesWithCtrlD(targetSession, keepPane, 24, 100*time.Millisecond); err != nil {
			return err
		}
	}
	if !shouldResetPaneAfterClear(keepCommand) {
		return nil
	}
	if err := runTmux("clear-history", "-t", keepPane); err != nil {
		return fmt.Errorf("clear-panes action (history): %w", err)
	}
	if err := runTmux("send-keys", "-R", "-t", keepPane); err != nil {
		return fmt.Errorf("clear-panes action (reset): %w", err)
	}
	return nil
}

func closeExtraPanesWithCtrlD(targetSession string, keepPane string, attempts int, delay time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		panes, err := listSessionPanes(targetSession)
		if err != nil {
			return err
		}
		extraPaneIDs := paneIDsExcludingKeep(panes, keepPane)
		if len(extraPaneIDs) == 0 {
			return nil
		}
		for _, paneID := range extraPaneIDs {
			if err := runTmux("send-keys", "-t", paneID, "C-d"); err != nil {
				return fmt.Errorf("clear-panes action (ctrl-d %s): %w", paneID, err)
			}
		}
		if i+1 < attempts {
			time.Sleep(delay)
		}
	}

	panes, err := listSessionPanes(targetSession)
	if err != nil {
		return err
	}
	extraPaneIDs := paneIDsExcludingKeep(panes, keepPane)
	if len(extraPaneIDs) > 0 {
		return fmt.Errorf("clear-panes action: could not close panes with ctrl-d: %s", strings.Join(extraPaneIDs, ", "))
	}
	return nil
}

func paneIDsExcludingKeep(panes []paneState, keepPane string) []string {
	extra := make([]string, 0)
	for _, pane := range panes {
		if pane.ID == "" || pane.ID == keepPane {
			continue
		}
		extra = append(extra, pane.ID)
	}
	return extra
}

func paneCommandByID(panes []paneState, paneID string) string {
	for _, pane := range panes {
		if pane.ID == paneID {
			return strings.TrimSpace(pane.Command)
		}
	}
	return ""
}

func shouldResetPaneAfterClear(command string) bool {
	return !isEditorCommand(command)
}

func openSlideAction(targetSession string, path string) error {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return errors.New("open-slide action requires path")
	}

	currentSlide, err := getSessionOption(targetSession, "@demo_it_slide")
	if err != nil {
		return err
	}
	if !shouldOpenSlide(currentSlide, trimmedPath) {
		return nil
	}

	panes, err := listSessionPanes(targetSession)
	if err != nil {
		return err
	}
	if paneTarget, ok := selectEditorPane(panes); ok {
		if err := runTmux("send-keys", "-t", paneTarget, "Escape"); err != nil {
			return fmt.Errorf("open-slide action (escape): %w", err)
		}
		if err := runTmux("send-keys", "-t", paneTarget, formatOpenSlideCommand(trimmedPath)); err != nil {
			return fmt.Errorf("open-slide action (command): %w", err)
		}
		if err := runTmux("send-keys", "-t", paneTarget, "Enter"); err != nil {
			return fmt.Errorf("open-slide action (enter): %w", err)
		}
	} else {
		paneTarget, ok := selectPaneForEditorStart(panes)
		if !ok {
			return errors.New("open-slide action requires at least one pane in the demo session")
		}
		if err := startEditorWithSlide(targetSession, paneTarget, trimmedPath); err != nil {
			return err
		}
	}
	if err := runTmux("set-option", "-t", targetSession, "-q", "@demo_it_slide", trimmedPath); err != nil {
		return fmt.Errorf("open-slide action (set metadata): %w", err)
	}
	return nil
}

func formatOpenSlideCommand(path string) string {
	escaped := strings.ReplaceAll(path, "'", "''")
	return ":execute 'edit ' . fnameescape('" + escaped + "') | silent! DemoItPresentationEnable"
}

func shouldOpenSlide(currentSlide string, nextSlide string) bool {
	current := strings.TrimSpace(currentSlide)
	next := strings.TrimSpace(nextSlide)
	if next == "" {
		return false
	}
	return current != next
}

func getSessionOption(targetSession string, option string) (string, error) {
	output, err := runTmuxOutput("show-options", "-qv", "-t", targetSession, option)
	if err != nil {
		return "", fmt.Errorf("read tmux option %q for %q: %w", option, targetSession, err)
	}
	return strings.TrimSpace(output), nil
}

func startEditorWithSlide(targetSession string, paneTarget string, slidePath string) error {
	workspace := ""
	if workspaceOption, err := getSessionOption(targetSession, "@demo_it_workspace"); err == nil {
		workspace = strings.TrimSpace(workspaceOption)
	}
	args := []string{"respawn-pane", "-k", "-t", paneTarget}
	if workspace != "" {
		args = append(args, "-c", workspace)
	}
	args = append(args, "nvim", "+silent! DemoItPresentationEnable", slidePath)
	if err := runTmux(args...); err != nil {
		return fmt.Errorf("open-slide action (start nvim): %w", err)
	}
	return nil
}

type paneState struct {
	ID      string
	Index   int
	Command string
	Active  bool
}

func listSessionPanes(targetSession string) ([]paneState, error) {
	output, err := runTmuxOutput("list-panes", "-t", targetSession, "-F", "#{pane_index}\t#{pane_id}\t#{pane_current_command}\t#{pane_active}")
	if err != nil {
		return nil, fmt.Errorf("list panes for %q: %w", targetSession, err)
	}
	panes := make([]paneState, 0)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, "\t", 4)
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		paneID := strings.TrimSpace(parts[1])
		if paneID == "" {
			continue
		}
		command := ""
		if len(parts) > 2 {
			command = strings.TrimSpace(parts[2])
		}
		active := false
		if len(parts) > 3 && strings.TrimSpace(parts[3]) == "1" {
			active = true
		}
		panes = append(panes, paneState{ID: paneID, Index: idx, Command: command, Active: active})
	}
	return panes, nil
}

func selectEditorPane(panes []paneState) (string, bool) {
	for _, pane := range panes {
		if pane.Active && isEditorCommand(pane.Command) {
			return pane.ID, true
		}
	}
	for _, pane := range panes {
		if isEditorCommand(pane.Command) {
			return pane.ID, true
		}
	}
	return "", false
}

func selectPaneForEditorStart(panes []paneState) (string, bool) {
	for _, pane := range panes {
		if pane.Active {
			return pane.ID, true
		}
	}
	if len(panes) == 0 {
		return "", false
	}
	return selectPaneToKeep(panes), true
}

func isEditorCommand(command string) bool {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "nvim", "vim", "vi":
		return true
	default:
		return false
	}
}

func selectPaneToKeep(panes []paneState) string {
	if len(panes) == 0 {
		return ""
	}
	keep := panes[0]
	for _, pane := range panes[1:] {
		if pane.Index < keep.Index {
			keep = pane
		}
	}
	return keep.ID
}

func ensureRunStarted(runID string, socketPath string) error {
	req := protocol.Request{
		ID:      fmt.Sprintf("cli-%d", time.Now().UnixNano()),
		Command: protocol.CommandStart,
		RunID:   runID,
	}

	c := client.SocketClient{SocketPath: socketPath}
	resp, err := c.Send(req)
	if err != nil {
		return fmt.Errorf("start run via daemon: %w", err)
	}
	if !resp.OK {
		if resp.Error == nil {
			return errors.New("daemon returned unknown error")
		}
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	return nil
}

type managedSession struct {
	Name         string
	Role         string
	Workspace    string
	Step         int
	LastAttached int64
}

type managedWorkspace struct {
	Display      string
	Workspace    string
	DemoSession  string
	NotesSession string
	SessionNames []string
	LastAttached int64
}

func setSessionMetadata(name string, workspace string, role string, step int) error {
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it", "1"); err != nil {
		return fmt.Errorf("mark tmux session %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_workspace", workspace); err != nil {
		return fmt.Errorf("set workspace metadata for %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_role", role); err != nil {
		return fmt.Errorf("set role metadata for %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_step", strconv.Itoa(step)); err != nil {
		return fmt.Errorf("set step metadata for %q: %w", name, err)
	}
	return nil
}

func ensureDemoNavigationBindings() error {
	if err := runTmux(
		"bind-key", "-T", "root", "C-s",
		"if-shell", "-F", "#{==:#{@demo_it},1}",
		"switch-client -T demo-it-nav",
		"send-keys C-s",
	); err != nil {
		return fmt.Errorf("bind demo-it prefix key: %w", err)
	}

	if err := runTmux(
		"bind-key", "-T", "demo-it-nav", "n",
		"run-shell", "-b", "demo-it next >/dev/null 2>&1",
		"\\;", "switch-client", "-T", "root",
	); err != nil {
		return fmt.Errorf("bind demo-it next key: %w", err)
	}

	if err := runTmux(
		"bind-key", "-T", "demo-it-nav", "p",
		"run-shell", "-b", "demo-it prev >/dev/null 2>&1",
		"\\;", "switch-client", "-T", "root",
	); err != nil {
		return fmt.Errorf("bind demo-it prev key: %w", err)
	}

	if err := runTmux("bind-key", "-T", "demo-it-nav", "C-s", "switch-client", "-T", "root"); err != nil {
		return fmt.Errorf("bind demo-it cancel key: %w", err)
	}
	if err := runTmux("bind-key", "-T", "demo-it-nav", "Escape", "switch-client", "-T", "root"); err != nil {
		return fmt.Errorf("bind demo-it escape key: %w", err)
	}

	return nil
}

func refreshNotesSession(notesSession string, workspacePath string, steps []transcript.Step, stepIndex int) error {
	notesPath := filepath.Join(workspacePath, ".demo-it", "notes.md")
	if err := os.MkdirAll(filepath.Dir(notesPath), 0o755); err != nil {
		return fmt.Errorf("create notes directory: %w", err)
	}
	if err := os.WriteFile(notesPath, []byte(renderSpeakerNotes(steps, stepIndex)), 0o644); err != nil {
		return fmt.Errorf("write notes file: %w", err)
	}
	if err := runTmux("respawn-pane", "-k", "-t", notesSession, "-c", workspacePath, "nvim", "+silent! DemoItPresentationEnable", notesPath); err != nil {
		return fmt.Errorf("refresh notes session %q: %w", notesSession, err)
	}
	return nil
}

func renderSpeakerNotes(steps []transcript.Step, stepIndex int) string {
	if len(steps) == 0 {
		return "No demo-it blocks found in demo-it.md.\n"
	}

	if stepIndex < 0 {
		stepIndex = 0
	}
	if stepIndex >= len(steps) {
		stepIndex = len(steps) - 1
	}

	notes := strings.TrimSpace(steps[stepIndex].SpeakerNotes)
	if notes == "" {
		notes = "No speaker notes for this step."
	}

	if stepIndex+1 < len(steps) {
		return notes + "\n\n---\nNext: " + steps[stepIndex+1].Title + "\n"
	}
	return notes + "\n"
}

type workspaceStepTransition struct {
	Available       bool
	StepIndex       int
	TotalSteps      int
	StepTitle       string
	ActionsExecuted bool
}

func advanceWorkspaceStepForCommand(command string) (workspaceStepTransition, error) {
	sessions, err := listManagedSessionDetails()
	if err != nil {
		return workspaceStepTransition{}, err
	}

	demoSession, notesSession, ok := selectWorkspaceSessions(sessions)
	if !ok {
		return workspaceStepTransition{}, nil
	}

	stepsPath := filepath.Join(demoSession.Workspace, "demo-it.md")
	steps, err := transcript.ParseStepsFile(stepsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workspaceStepTransition{}, nil
		}
		return workspaceStepTransition{}, fmt.Errorf("parse %s: %w", stepsPath, err)
	}
	if len(steps) == 0 {
		return workspaceStepTransition{}, nil
	}

	currentStep := normalizeStepIndex(demoSession.Step, len(steps))
	transition := workspaceStepTransition{
		Available:  true,
		StepIndex:  currentStep,
		TotalSteps: len(steps),
		StepTitle:  stepTitleAt(steps, currentStep),
	}

	if command == "rerun" {
		if demoSession.Step < 0 || demoSession.Step >= len(steps) {
			return transition, nil
		}
		if err := runActions(demoSession.Name, steps[demoSession.Step].Actions); err != nil {
			return workspaceStepTransition{}, err
		}
		transition.StepIndex = demoSession.Step
		transition.StepTitle = stepTitleAt(steps, demoSession.Step)
		transition.ActionsExecuted = true
		return transition, nil
	}

	nextStep, shouldExecuteActions := resolveStepTransition(demoSession.Step, len(steps), command)
	if nextStep == demoSession.Step {
		return transition, nil
	}

	if shouldExecuteActions {
		if err := runActions(demoSession.Name, steps[nextStep].Actions); err != nil {
			return workspaceStepTransition{}, err
		}
	}
	if err := setSessionMetadata(demoSession.Name, demoSession.Workspace, "demo", nextStep); err != nil {
		return workspaceStepTransition{}, err
	}
	if notesSession != nil {
		if err := setSessionMetadata(notesSession.Name, demoSession.Workspace, "notes", nextStep); err != nil {
			return workspaceStepTransition{}, err
		}
		if err := refreshNotesSession(notesSession.Name, demoSession.Workspace, steps, nextStep); err != nil {
			return workspaceStepTransition{}, err
		}
	}

	transition.StepIndex = normalizeStepIndex(nextStep, len(steps))
	transition.StepTitle = stepTitleAt(steps, transition.StepIndex)
	transition.ActionsExecuted = shouldExecuteActions
	return transition, nil
}

func normalizeStepIndex(step int, totalSteps int) int {
	if totalSteps == 0 {
		return -1
	}
	if step < 0 {
		return 0
	}
	if step >= totalSteps {
		return totalSteps - 1
	}
	return step
}

func stepTitleAt(steps []transcript.Step, index int) string {
	if index < 0 || index >= len(steps) {
		return ""
	}
	return strings.TrimSpace(steps[index].Title)
}

func mergeWorkspaceTransition(state interface{}, transition workspaceStepTransition) (map[string]interface{}, error) {
	bytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encode state: %w", err)
	}
	merged := map[string]interface{}{}
	if err := json.Unmarshal(bytes, &merged); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}

	merged["workspace_step_available"] = transition.Available
	merged["workspace_step"] = transition.StepIndex
	merged["workspace_total_steps"] = transition.TotalSteps
	if transition.StepTitle != "" {
		merged["workspace_step_title"] = transition.StepTitle
	}
	merged["workspace_actions_executed"] = transition.ActionsExecuted
	return merged, nil
}

func resolveStepTransition(currentStep int, totalSteps int, command string) (int, bool) {
	if totalSteps == 0 {
		return currentStep, false
	}
	if currentStep < 0 {
		currentStep = 0
	}
	if currentStep >= totalSteps {
		currentStep = totalSteps - 1
	}

	switch command {
	case "next":
		next := currentStep + 1
		if next >= totalSteps {
			return currentStep, false
		}
		return next, true
	case "prev":
		prev := currentStep - 1
		if prev < 0 {
			return currentStep, false
		}
		return prev, false
	default:
		return currentStep, false
	}
}

func selectWorkspaceSessions(sessions []managedSession) (managedSession, *managedSession, bool) {
	workspaces := groupManagedWorkspaces(sessions)
	if len(workspaces) == 0 {
		return managedSession{}, nil, false
	}
	workspace := workspaces[0]
	if workspace.DemoSession == "" {
		return managedSession{}, nil, false
	}

	var demo managedSession
	foundDemo := false
	var notes *managedSession
	for _, session := range sessions {
		if session.Name == workspace.DemoSession {
			demo = session
			foundDemo = true
		}
		if workspace.NotesSession != "" && session.Name == workspace.NotesSession {
			s := session
			notes = &s
		}
	}
	if !foundDemo {
		return managedSession{}, nil, false
	}
	return demo, notes, true
}

func listSessionsCommand() error {
	workspaces, err := listManagedWorkspaces()
	if err != nil {
		return err
	}
	if len(workspaces) == 0 {
		fmt.Println("no demo-it tmux sessions")
		return nil
	}
	for idx, workspace := range workspaces {
		fmt.Printf("%d\t%s\n", idx+1, workspace.Display)
	}
	return nil
}

func notesCommand() error {
	workspaces, err := listManagedWorkspaces()
	if err != nil {
		return err
	}
	if len(workspaces) == 0 {
		return errors.New("no demo-it tmux sessions")
	}

	notesSession := latestWorkspaceSessionByRole(workspaces, "notes")
	if notesSession == "" {
		return errors.New("no demo-it notes session found")
	}

	if os.Getenv("TMUX") != "" {
		return runTmuxWithStdio("switch-client", "-t", notesSession)
	}
	return runTmuxWithStdio("attach-session", "-t", notesSession)
}

func showCommand() error {
	workspaces, err := listManagedWorkspaces()
	if err != nil {
		return err
	}
	if len(workspaces) == 0 {
		return errors.New("no demo-it tmux sessions")
	}

	demoSession := latestWorkspaceSessionByRole(workspaces, "demo")
	if demoSession == "" {
		return errors.New("no demo-it demo session found")
	}

	if os.Getenv("TMUX") != "" {
		return runTmuxWithStdio("switch-client", "-t", demoSession)
	}
	return runTmuxWithStdio("attach-session", "-t", demoSession)
}

func latestWorkspaceSessionByRole(workspaces []managedWorkspace, role string) string {
	for _, workspace := range workspaces {
		switch role {
		case "demo":
			if workspace.DemoSession != "" {
				return workspace.DemoSession
			}
		case "notes":
			if workspace.NotesSession != "" {
				return workspace.NotesSession
			}
		}
	}
	return ""
}

func killSessionsCommand(rawArgs []string) error {
	workspaces, err := listManagedWorkspaces()
	if err != nil {
		return err
	}
	if len(workspaces) == 0 {
		fmt.Println("no demo-it tmux sessions")
		return nil
	}

	selected, err := selectWorkspacesToKill(workspaces, rawArgs)
	if err != nil {
		return err
	}

	killedCount := 0
	seen := map[string]struct{}{}
	for _, workspace := range selected {
		for _, session := range workspace.SessionNames {
			if _, ok := seen[session]; ok {
				continue
			}
			seen[session] = struct{}{}
			if err := runTmux("kill-session", "-t", session); err != nil {
				return fmt.Errorf("kill tmux session %q: %w", session, err)
			}
			killedCount++
		}
	}
	fmt.Printf("killed %d demo-it tmux session(s)\n", killedCount)
	return nil
}

func selectWorkspacesToKill(workspaces []managedWorkspace, rawArgs []string) ([]managedWorkspace, error) {
	if len(rawArgs) == 0 {
		return workspaces, nil
	}

	selected := make([]managedWorkspace, 0, len(rawArgs))
	seen := map[int]struct{}{}
	for _, arg := range rawArgs {
		idx, err := strconv.Atoi(arg)
		if err != nil {
			return nil, fmt.Errorf("kill expects numeric session indexes from 'demo-it list', got %q", arg)
		}
		if idx < 1 || idx > len(workspaces) {
			return nil, fmt.Errorf("kill index out of range: %d (valid: 1..%d)", idx, len(workspaces))
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		selected = append(selected, workspaces[idx-1])
	}
	return selected, nil
}

func listManagedWorkspaces() ([]managedWorkspace, error) {
	sessions, err := listManagedSessionDetails()
	if err != nil {
		return nil, err
	}
	return groupManagedWorkspaces(sessions), nil
}

func groupManagedWorkspaces(sessions []managedSession) []managedWorkspace {
	groups := map[string]*managedWorkspace{}
	for _, session := range sessions {
		key := workspaceKey(session)
		group, ok := groups[key]
		if !ok {
			group = &managedWorkspace{
				Workspace: key,
			}
			groups[key] = group
		}

		if group.Workspace == "" && session.Workspace != "" {
			group.Workspace = session.Workspace
		}
		if session.LastAttached > group.LastAttached {
			group.LastAttached = session.LastAttached
		}

		if session.Role == "demo" {
			group.DemoSession = session.Name
		}
		if session.Role == "notes" {
			group.NotesSession = session.Name
		}

		exists := false
		for _, name := range group.SessionNames {
			if name == session.Name {
				exists = true
				break
			}
		}
		if !exists {
			group.SessionNames = append(group.SessionNames, session.Name)
		}
	}

	workspaces := make([]managedWorkspace, 0, len(groups))
	for _, group := range groups {
		if group.DemoSession == "" {
			for _, name := range group.SessionNames {
				if strings.HasSuffix(name, "-demo") {
					group.DemoSession = name
					break
				}
			}
		}
		if group.NotesSession == "" {
			for _, name := range group.SessionNames {
				if strings.HasSuffix(name, "-notes") {
					group.NotesSession = name
					break
				}
			}
		}
		if group.DemoSession != "" {
			group.Display = group.DemoSession
		} else if len(group.SessionNames) > 0 {
			group.Display = group.SessionNames[0]
		}
		workspaces = append(workspaces, *group)
	}

	sort.Slice(workspaces, func(i, j int) bool {
		if workspaces[i].LastAttached == workspaces[j].LastAttached {
			return workspaces[i].Display < workspaces[j].Display
		}
		return workspaces[i].LastAttached > workspaces[j].LastAttached
	})

	return workspaces
}

func workspaceKey(session managedSession) string {
	if session.Workspace != "" {
		return session.Workspace
	}
	if strings.HasSuffix(session.Name, "-demo") {
		return strings.TrimSuffix(session.Name, "-demo")
	}
	if strings.HasSuffix(session.Name, "-notes") {
		return strings.TrimSuffix(session.Name, "-notes")
	}
	return session.Name
}

func listManagedSessionDetails() ([]managedSession, error) {
	output, err := runTmuxOutput("list-sessions", "-F", "#{session_name}\t#{@demo_it}\t#{@demo_it_role}\t#{@demo_it_workspace}\t#{@demo_it_step}\t#{session_last_attached}")
	if err != nil {
		if isTmuxNoServerError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list tmux sessions: %w", err)
	}

	sessions := make([]managedSession, 0)
	seen := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, "\t", 6)
		name := strings.TrimSpace(parts[0])
		marker := ""
		role := ""
		workspace := ""
		step := -1
		lastAttached := int64(0)
		if len(parts) > 1 {
			marker = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			role = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			workspace = strings.TrimSpace(parts[3])
		}
		if len(parts) > 4 {
			if parsed, err := strconv.Atoi(strings.TrimSpace(parts[4])); err == nil {
				step = parsed
			}
		}
		if len(parts) > 5 {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(parts[5]), 10, 64); err == nil {
				lastAttached = parsed
			}
		}

		if marker != "1" && !isLegacyDemoSession(name) {
			continue
		}
		if role == "" {
			if strings.HasSuffix(name, "-notes") {
				role = "notes"
			} else {
				role = "demo"
			}
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		sessions = append(sessions, managedSession{
			Name:         name,
			Role:         role,
			Workspace:    workspace,
			Step:         step,
			LastAttached: lastAttached,
		})
	}

	return sessions, nil
}

func isLegacyDemoSession(name string) bool {
	return strings.HasSuffix(name, "-demo") || strings.HasSuffix(name, "-notes")
}

func isTmuxNoServerError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "no server running") || strings.Contains(message, "failed to connect to server")
}

func tmuxSessionExists(name string) (bool, error) {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("check tmux session %q: %w", name, err)
	}
	return true, nil
}

func runTmux(args ...string) error {
	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func runTmuxOutput(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %v: %w: %s", args, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func runTmuxWithStdio(args ...string) error {
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux %v: %w", args, err)
	}
	return nil
}

func isProtocolCommand(name string) bool {
	switch name {
	case "start", "status", "reload", "next", "prev", "rerun", "jump", "focus":
		return true
	default:
		return false
	}
}

func parseSubcommand(name string, rawArgs []string) (protocol.Command, json.RawMessage, error) {
	switch name {
	case "start":
		return protocol.CommandStart, nil, nil
	case "status":
		return protocol.CommandStatus, nil, nil
	case "reload":
		return protocol.CommandReload, nil, nil
	case "next":
		return parseNextArgs(rawArgs, protocol.CommandNext)
	case "prev":
		return protocol.CommandPrev, nil, nil
	case "rerun":
		return parseNextArgs(rawArgs, protocol.CommandRerun)
	case "jump":
		return parseJumpArgs(rawArgs)
	case "focus":
		return parseFocusArgs(rawArgs)
	default:
		return "", nil, fmt.Errorf("unknown subcommand %q\n%s", name, usage)
	}
}

func parseNextArgs(rawArgs []string, command protocol.Command) (protocol.Command, json.RawMessage, error) {
	fs := flag.NewFlagSet(string(command), flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(rawArgs); err != nil {
		return "", nil, err
	}
	if fs.NArg() > 0 {
		return "", nil, fmt.Errorf("%s does not accept positional arguments", command)
	}

	encoded, err := json.Marshal(protocol.NextArgs{})
	if err != nil {
		return "", nil, fmt.Errorf("encode args: %w", err)
	}
	return command, encoded, nil
}

func parseJumpArgs(rawArgs []string) (protocol.Command, json.RawMessage, error) {
	fs := flag.NewFlagSet("jump", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	slide := fs.String("slide", "", "slide id or zero-based slide index")
	if err := fs.Parse(rawArgs); err != nil {
		return "", nil, err
	}

	if *slide == "" {
		return "", nil, fmt.Errorf("jump requires --slide <id|index>")
	}

	args := protocol.JumpArgs{}
	if idx, err := strconv.Atoi(*slide); err == nil {
		args.SlideIndex = &idx
	} else {
		args.SlideID = *slide
	}

	encoded, err := json.Marshal(args)
	if err != nil {
		return "", nil, fmt.Errorf("encode args: %w", err)
	}
	return protocol.CommandJump, encoded, nil
}

func parseFocusArgs(rawArgs []string) (protocol.Command, json.RawMessage, error) {
	fs := flag.NewFlagSet("focus", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	policy := fs.String("policy", "", "focus policy: present|return|none")
	if err := fs.Parse(rawArgs); err != nil {
		return "", nil, err
	}
	if *policy == "" {
		return "", nil, fmt.Errorf("focus requires --policy <present|return|none>")
	}

	args := protocol.SetFocusPolicyArgs{Focus: protocol.FocusPolicy(*policy)}

	encoded, err := json.Marshal(args)

	if err != nil {
		return "", nil, fmt.Errorf("encode args: %w", err)
	}
	return protocol.CommandSetFocusPolicy, encoded, nil
}

func usageError() error {
	return fmt.Errorf("missing command or workspace path\n%s", usage)
}

const usage = `Usage:
  demo-it [--run-id <id>] [--socket <path>] <command> [flags]
  demo-it <workspace-path>

Commands:
  start [workspace-path]
  status
  reload
  next
  prev
  rerun
  jump --slide <id|index>
  focus --policy <present|return|none>
  list
  notes
  show
  kill [session-index ...]

Start behavior:
  start (without a path) bootstraps the current working directory.
  start <workspace-path> bootstraps that workspace.
  start requires <workspace>/demo-it.md.

Workspace mode:
  Passing a workspace path directly is equivalent to start <workspace-path>.

Session lifecycle:
  demo-it list               # show numbered managed workspaces (demo session)
  demo-it notes              # open notes session for latest workspace
  demo-it show               # open demo session for latest workspace
  demo-it kill               # kill all managed sessions
  demo-it kill 1 3           # kill selected workspace indexes from list
`
