package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dejanr/demo-it/internal/client"
	"github.com/dejanr/demo-it/internal/protocol"
	"github.com/dejanr/demo-it/internal/runctx"
	"github.com/dejanr/demo-it/internal/transcript"
	"gopkg.in/yaml.v3"
)

var debugLogFile *os.File

var traceAnsiOSC = regexp.MustCompile(`\x1b\][^\x1b\x07]*(?:\x07|\x1b\\)`)
var traceAnsiDCS = regexp.MustCompile(`\x1bP(?s:.*?)(?:\x1b\\)`)
var traceAnsiCSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var traceAnsiSingle = regexp.MustCompile(`\x1b[@-_]`)

func initDebugLog() error {
	path := strings.TrimSpace(os.Getenv("DEMO_IT_DEBUG_LOG"))
	if path == "" {
		return nil
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open debug log %q: %w", path, err)
	}
	debugLogFile = file
	debugf("debug enabled pid=%d argv=%q", os.Getpid(), os.Args)
	return nil
}

func closeDebugLog() {
	if debugLogFile == nil {
		return
	}
	_ = debugLogFile.Close()
	debugLogFile = nil
}

func debugf(format string, args ...interface{}) {
	if debugLogFile == nil {
		return
	}
	line := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(debugLogFile, "%s %s\n", time.Now().Format(time.RFC3339Nano), line)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if err := initDebugLog(); err != nil {
		return err
	}
	defer closeDebugLog()
	debugf("run start")

	repoRoot, err := runctx.RepoRoot()
	if err != nil {
		return err
	}

	defaultRunID := runctx.DefaultRunID(repoRoot)
	defaultSocketPath := runctx.DefaultSocketPath(repoRoot)

	runID := flag.String("run-id", defaultRunID, "run identifier")
	socketPath := flag.String("socket", defaultSocketPath, "daemon unix socket path")
	flag.Parse()
	debugf("parsed flags run_id=%q socket=%q args=%q", *runID, *socketPath, flag.Args())

	if flag.NArg() == 0 {
		return usageError()
	}

	target := flag.Arg(0)
	debugf("dispatch target=%q", target)
	switch target {
	case "status":
		return statusCommand(*runID, *socketPath)
	case "run-status":
		return runProtocolCommand("status", flag.Args()[1:], *runID, *socketPath)
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
	case "record":
		return recordCommand(flag.Args()[1:])
	case "__record-event":
		return recordEventCommand(flag.Args()[1:])
	case "__record-split-event":
		return recordSplitEventCommand(flag.Args()[1:])
	case "__record-shell":
		return recordShellCommand(flag.Args()[1:])
	case "__bootstrap-step":
		return bootstrapStepCommand(flag.Args()[1:])
	case "trace-next":
		return traceNextCommand(*runID, *socketPath)
	}

	if !isProtocolCommand(target) {
		return bootstrapWorkspace(target, *runID, *socketPath, false)
	}

	return runProtocolCommand(target, flag.Args()[1:], *runID, *socketPath)
}

func runProtocolCommand(target string, rawArgs []string, runID string, socketPath string) error {
	cmd, args, err := parseSubcommand(target, rawArgs)
	if err != nil {
		return err
	}

	req := protocol.Request{
		ID:      fmt.Sprintf("cli-%d", time.Now().UnixNano()),
		Command: cmd,
		RunID:   runID,
		Args:    args,
	}

	c := client.SocketClient{SocketPath: socketPath}
	resp, err := sendWithAutoStart(c, req, target, runID, socketPath)
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
		workspaceTransition, err = advanceWorkspaceStepForCommand(target, runID, socketPath)
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
		return protocol.Response{}, wrapLocalDaemonUnavailable(err, socketPath)
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
		return protocol.Response{}, wrapLocalDaemonUnavailable(err, socketPath)
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

func wrapLocalDaemonUnavailable(err error, socketPath string) error {
	if err == nil {
		return nil
	}
	if !localDaemonRequired() || !isDaemonConnectionError(err) {
		return err
	}
	return fmt.Errorf("local demo-it daemon is not running on %s; run 'devenv up' first", strings.TrimSpace(socketPath))
}

func localDaemonRequired() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("DEMO_IT_REQUIRE_LOCAL_DAEMON")))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isDaemonConnectionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "dial unix") && !strings.Contains(message, "connect") {
		return false
	}
	return strings.Contains(message, "no such file or directory") || strings.Contains(message, "connection refused") || strings.Contains(message, "connection reset") || strings.Contains(message, "broken pipe")
}

type bootstrapTarget struct {
	WorkspacePath  string
	TranscriptPath string
}

func resolveBootstrapTarget(rawPath string) (bootstrapTarget, error) {
	absolutePath, err := filepath.Abs(rawPath)
	if err != nil {
		return bootstrapTarget{}, fmt.Errorf("resolve workspace path: %w", err)
	}

	stat, err := os.Stat(absolutePath)
	if err != nil {
		return bootstrapTarget{}, fmt.Errorf("workspace path: %w", err)
	}
	if stat.IsDir() {
		return bootstrapTarget{
			WorkspacePath:  absolutePath,
			TranscriptPath: filepath.Join(absolutePath, "demo-it.md"),
		}, nil
	}
	if stat.Mode().IsRegular() {
		return bootstrapTarget{
			WorkspacePath:  filepath.Dir(absolutePath),
			TranscriptPath: absolutePath,
		}, nil
	}

	return bootstrapTarget{}, fmt.Errorf("workspace path must be a directory or file: %s", absolutePath)
}

func bootstrapWorkspace(rawPath string, runID string, socketPath string, requireTranscript bool) error {
	target, err := resolveBootstrapTarget(rawPath)
	if err != nil {
		return err
	}
	workspacePath := target.WorkspacePath
	transcriptPath := target.TranscriptPath

	demoSession := runctx.DemoSessionName(workspacePath)
	notesSession := runctx.NotesSessionName(workspacePath)

	workspaces, err := listManagedWorkspaces()
	if err != nil {
		return err
	}
	if _, err := killManagedWorkspaceSessions(workspaces); err != nil {
		return err
	}

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

	steps, currentStep, err := executeBootstrapStep(transcriptPath, requireTranscript)
	if err != nil {
		return err
	}

	if err := setSessionMetadata(demoSession, workspacePath, transcriptPath, "demo", currentStep); err != nil {
		return err
	}
	if err := setSessionMetadata(notesSession, workspacePath, transcriptPath, "notes", currentStep); err != nil {
		return err
	}
	if err := setSessionRuntimeContext(demoSession, runID, socketPath); err != nil {
		return err
	}
	if err := setSessionRuntimeContext(notesSession, runID, socketPath); err != nil {
		return err
	}

	notesText := renderSpeakerNotes(steps, currentStep)
	if err := syncNotesWithDaemon(notesText, runID, socketPath); err != nil {
		return err
	}
	if err := refreshNotesSession(notesSession, workspacePath, notesText); err != nil {
		return err
	}

	if err := ensureRunStarted(runID, socketPath); err != nil {
		return err
	}
	cliPath, err := resolveCLIPath()
	if err != nil {
		return err
	}
	debugf("resolved cli path=%q", cliPath)
	if err := setTmuxRuntimeEnvironment(cliPath, runID, socketPath); err != nil {
		return err
	}
	if err := ensureDemoNavigationBindings(cliPath, strings.TrimSpace(os.Getenv("DEMO_IT_DEBUG_LOG"))); err != nil {
		return err
	}
	if err := syncAutoNextWithDaemon(steps, currentStep, runID, socketPath); err != nil {
		return err
	}
	if err := scheduleBootstrapStepPlayback(cliPath, demoSession, notesSession, workspacePath, transcriptPath, currentStep, 500*time.Millisecond); err != nil {
		return err
	}

	if os.Getenv("TMUX") != "" {
		if err := runTmux("switch-client", "-t", demoSession); err != nil {
			if isTmuxNoCurrentClientError(err) {
				return nil
			}
			return err
		}
		return nil
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

func executeBootstrapStep(transcriptPath string, requireTranscript bool) ([]transcript.Step, int, error) {
	stepsPath := strings.TrimSpace(transcriptPath)
	if stepsPath == "" {
		return nil, -1, errors.New("bootstrap transcript path is empty")
	}
	debugf("bootstrap parse steps path=%q", stepsPath)
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
		debugf("bootstrap parsed 0 steps")
		return steps, -1, nil
	}
	debugf("bootstrap parsed steps=%d first_title=%q", len(steps), steps[0].Title)
	return steps, 0, nil
}

func scheduleBootstrapStepPlayback(cliPath string, demoSession string, notesSession string, workspacePath string, transcriptPath string, stepIndex int, delay time.Duration) error {
	if stepIndex < 0 {
		return nil
	}
	delayMS := delay.Milliseconds()
	if delayMS < 0 {
		delayMS = 0
	}
	command := fmt.Sprintf(
		"sleep %s; DEMO_IT_DEBUG_LOG=%s %s __bootstrap-step --session %s --notes-session %s --workspace %s --transcript %s --step %d --expected-step %d",
		shellQuote(fmt.Sprintf("%.3f", float64(delayMS)/1000.0)),
		shellQuote(strings.TrimSpace(os.Getenv("DEMO_IT_DEBUG_LOG"))),
		shellQuote(cliPath),
		shellQuote(demoSession),
		shellQuote(notesSession),
		shellQuote(workspacePath),
		shellQuote(transcriptPath),
		stepIndex,
		stepIndex,
	)
	if err := runTmux("run-shell", "-b", command); err != nil {
		return fmt.Errorf("schedule bootstrap step playback: %w", err)
	}
	return nil
}

func bootstrapStepCommand(rawArgs []string) error {
	fs := flag.NewFlagSet("__bootstrap-step", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	sessionName := fs.String("session", "", "demo tmux session name")
	notesSessionName := fs.String("notes-session", "", "notes tmux session name")
	workspacePath := fs.String("workspace", "", "workspace path")
	transcriptPath := fs.String("transcript", "", "transcript path")
	stepIndex := fs.Int("step", 0, "step index to execute")
	expectedStep := fs.Int("expected-step", -1, "expected current step before execution")
	if err := fs.Parse(rawArgs); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("__bootstrap-step does not accept positional arguments")
	}

	demoSession := strings.TrimSpace(*sessionName)
	if demoSession == "" {
		return errors.New("__bootstrap-step requires --session")
	}
	transcriptFile := strings.TrimSpace(*transcriptPath)
	if transcriptFile == "" {
		return errors.New("__bootstrap-step requires --transcript")
	}
	workspace := strings.TrimSpace(*workspacePath)
	if workspace == "" {
		workspace = filepath.Dir(transcriptFile)
	}

	exists, err := tmuxSessionExists(demoSession)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	if *expectedStep >= 0 {
		currentStep, err := sessionStepValue(demoSession)
		if err != nil {
			return nil
		}
		if currentStep != *expectedStep {
			return nil
		}
	}

	steps, err := transcript.ParseStepsFile(transcriptFile)
	if err != nil {
		return fmt.Errorf("parse %s: %w", transcriptFile, err)
	}
	if *stepIndex < 0 || *stepIndex >= len(steps) {
		return nil
	}
	if err := runActions(demoSession, steps[*stepIndex].Actions); err != nil {
		return err
	}
	if err := setSessionMetadata(demoSession, workspace, transcriptFile, "demo", *stepIndex); err != nil {
		return err
	}
	resolvedNotesSession := strings.TrimSpace(*notesSessionName)
	if resolvedNotesSession != "" {
		notesExists, err := tmuxSessionExists(resolvedNotesSession)
		if err != nil {
			return err
		}
		if notesExists {
			if err := setSessionMetadata(resolvedNotesSession, workspace, transcriptFile, "notes", *stepIndex); err != nil {
				return err
			}
		}
	}
	return nil
}

func sessionStepValue(sessionName string) (int, error) {
	value, err := getSessionOption(sessionName, "@demo_it_step")
	if err != nil {
		return -1, err
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return -1, err
	}
	return parsed, nil
}

func runActions(targetSession string, actions []transcript.Action) error {
	for idx, action := range actions {
		debugf("action start session=%q index=%d kind=%q pane=%q", targetSession, idx, action.Kind, action.Pane)
		switch action.Kind {
		case "insert-text":
			targetPane, err := resolveActionTarget(targetSession, action.Pane)
			if err != nil {
				return fmt.Errorf("insert-text action: %w", err)
			}
			if err := runTmux("send-keys", "-t", targetPane, action.Text); err != nil {
				return fmt.Errorf("insert-text action: %w", err)
			}
		case "key":
			targetPane, err := resolveActionTarget(targetSession, action.Pane)
			if err != nil {
				return fmt.Errorf("key action: %w", err)
			}
			if err := runTmux("send-keys", "-t", targetPane, tmuxKey(action.Key)); err != nil {
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
		case "killall-pane":
			if err := killAllPaneAction(targetSession); err != nil {
				return err
			}
		case "kill-pane":
			if err := killPaneAction(targetSession); err != nil {
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
		debugf("action done session=%q index=%d kind=%q", targetSession, idx, action.Kind)
	}
	return nil
}

func tmuxKey(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "enter", "return":
		return "Enter"
	case "escape", "esc":
		return "Escape"
	case "tab":
		return "Tab"
	case "space":
		return "Space"
	case "backspace", "bspace", "bs":
		return "BSpace"
	default:
		return key
	}
}

func resolveActionTarget(targetSession string, paneSelector string) (string, error) {
	selector := strings.TrimSpace(paneSelector)
	if selector == "" {
		debugf("resolve action target session=%q selector=default -> %q", targetSession, targetSession)
		return targetSession, nil
	}

	panes, err := listSessionPanes(targetSession)
	if err != nil {
		return "", err
	}
	targetPane, ok := selectPaneBySelector(panes, selector)
	if !ok {
		debugf("resolve action target session=%q selector=%q failed panes=%v", targetSession, selector, panes)
		return "", fmt.Errorf("pane selector %q not found", selector)
	}
	debugf("resolve action target session=%q selector=%q -> %q", targetSession, selector, targetPane)
	return targetPane, nil
}

func selectPaneBySelector(panes []paneState, selector string) (string, bool) {
	if len(panes) == 0 {
		return "", false
	}

	trimmed := strings.TrimSpace(selector)
	if trimmed == "" {
		return "", false
	}

	for _, pane := range panes {
		if pane.ID == trimmed {
			return pane.ID, true
		}
	}

	if idx, err := strconv.Atoi(trimmed); err == nil {
		for _, pane := range panes {
			if pane.Index == idx {
				return pane.ID, true
			}
		}
		return "", false
	}

	switch strings.ToLower(trimmed) {
	case "active":
		for _, pane := range panes {
			if pane.Active {
				return pane.ID, true
			}
		}
		return "", false
	case "last", "newest", "right":
		selected := panes[0]
		for _, pane := range panes[1:] {
			if pane.Index > selected.Index {
				selected = pane
			}
		}
		return selected.ID, true
	case "first", "left":
		selected := panes[0]
		for _, pane := range panes[1:] {
			if pane.Index < selected.Index {
				selected = pane
			}
		}
		return selected.ID, true
	default:
		return "", false
	}
}

type keyMacroPlaybackStep struct {
	Key        string
	DelayAfter time.Duration
}

func keyMacroAction(targetSession string, action transcript.Action) error {
	targetPane, err := resolveActionTarget(targetSession, action.Pane)
	if err != nil {
		return fmt.Errorf("key-macro action: %w", err)
	}

	playback, err := keyMacroPlayback(action)
	if err != nil {
		return fmt.Errorf("key-macro action: %w", err)
	}
	debugf("key-macro target=%q steps=%d", targetPane, len(playback))
	if action.DelayMS != nil && *action.DelayMS > 0 {
		time.Sleep(time.Duration(*action.DelayMS) * time.Millisecond)
	}

	for i, step := range playback {
		key := tmuxKey(step.Key)
		debugf("key-macro step=%d key=%q delay_after=%s", i, key, step.DelayAfter)
		if err := runTmux("send-keys", "-t", targetPane, key); err != nil {
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

	if action.DelayMS != nil && *action.DelayMS < 0 {
		return nil, errors.New("delay_ms must be >= 0")
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

func killPaneAction(targetSession string) error {
	panes, err := listSessionPanes(targetSession)
	if err != nil {
		return err
	}
	if len(panes) <= 1 {
		return nil
	}

	targetPane, ok := selectPaneBySelector(panes, "last")
	if !ok || strings.TrimSpace(targetPane) == "" {
		return errors.New("kill-pane action: could not resolve latest pane")
	}
	if err := runTmux("kill-pane", "-t", targetPane); err != nil {
		return fmt.Errorf("kill-pane action: %w", err)
	}
	return nil
}

func killAllPaneAction(targetSession string) error {
	panes, err := listSessionPanes(targetSession)
	if err != nil {
		return err
	}
	if len(panes) <= 1 {
		return nil
	}

	keepPane := selectPaneToKeep(panes)
	extraPaneIDs := paneIDsExcludingKeep(panes, keepPane)
	for _, paneID := range extraPaneIDs {
		if err := runTmux("kill-pane", "-t", paneID); err != nil {
			return fmt.Errorf("killall-pane action: %w", err)
		}
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

func shellQuote(value string) string {
	escaped := strings.ReplaceAll(value, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

func resolveCLIPath() (string, error) {
	override := strings.TrimSpace(os.Getenv("DEMO_IT_PATH"))
	if override != "" {
		if strings.ContainsRune(override, os.PathSeparator) || strings.HasPrefix(override, ".") {
			path, err := filepath.Abs(override)
			if err != nil {
				return "", fmt.Errorf("resolve DEMO_IT_PATH %q: %w", override, err)
			}
			if resolved, err := exec.LookPath(path); err == nil {
				return resolved, nil
			}
			return "", fmt.Errorf("resolve DEMO_IT_PATH %q: not executable", override)
		}
		path, err := exec.LookPath(override)
		if err != nil {
			return "", fmt.Errorf("resolve DEMO_IT_PATH %q via PATH: %w", override, err)
		}
		return path, nil
	}

	path, err := exec.LookPath("demo-it")
	if err != nil {
		return "", fmt.Errorf("resolve demo-it via PATH: %w", err)
	}
	return path, nil
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
		return fmt.Errorf("start run via daemon: %w", wrapLocalDaemonUnavailable(err, socketPath))
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
	Name           string
	Role           string
	Workspace      string
	TranscriptPath string
	Step           int
	LastAttached   int64
}

type managedWorkspace struct {
	Display      string
	Workspace    string
	DemoSession  string
	NotesSession string
	SessionNames []string
	LastAttached int64
}

func setSessionMetadata(name string, workspace string, transcriptPath string, role string, step int) error {
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it", "1"); err != nil {
		return fmt.Errorf("mark tmux session %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_workspace", workspace); err != nil {
		return fmt.Errorf("set workspace metadata for %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_transcript", transcriptPath); err != nil {
		return fmt.Errorf("set transcript metadata for %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_role", role); err != nil {
		return fmt.Errorf("set role metadata for %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_step", strconv.Itoa(step)); err != nil {
		return fmt.Errorf("set step metadata for %q: %w", name, err)
	}
	return nil
}

func setSessionRuntimeContext(name string, runID string, socketPath string) error {
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_run_id", runID); err != nil {
		return fmt.Errorf("set run_id metadata for %q: %w", name, err)
	}
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_socket", socketPath); err != nil {
		return fmt.Errorf("set socket metadata for %q: %w", name, err)
	}
	debugLogPath := strings.TrimSpace(os.Getenv("DEMO_IT_DEBUG_LOG"))
	if err := runTmux("set-option", "-t", name, "-q", "@demo_it_debug_log", debugLogPath); err != nil {
		return fmt.Errorf("set debug log metadata for %q: %w", name, err)
	}
	return nil
}

func setTmuxRuntimeEnvironment(cliPath string, runID string, socketPath string) error {
	if err := runTmux("set-environment", "-g", "DEMO_IT_PATH", cliPath); err != nil {
		return fmt.Errorf("set tmux DEMO_IT_PATH: %w", err)
	}
	if err := runTmux("set-environment", "-g", "DEMO_IT_RUN_ID", runID); err != nil {
		return fmt.Errorf("set tmux DEMO_IT_RUN_ID: %w", err)
	}
	if err := runTmux("set-environment", "-g", "DEMO_IT_SOCKET", socketPath); err != nil {
		return fmt.Errorf("set tmux DEMO_IT_SOCKET: %w", err)
	}
	return nil
}

func ensureDemoNavigationBindings(cliPath string, debugLogPath string) error {
	if err := runTmux(
		"bind-key", "-T", "root", "C-s",
		"if-shell", "-F", "#{==:#{@demo_it},1}",
		"switch-client -T demo-it-nav",
		"send-keys C-s",
	); err != nil {
		return fmt.Errorf("bind demo-it prefix key: %w", err)
	}

	nextCommand := fmt.Sprintf(
		"__out=$(DEMO_IT_DEBUG_LOG=%s %s next 2>&1); __code=$?; if [ \"$__code\" -ne 0 ]; then __msg=$(printf '%%s' \"$__out\" | tr '\\n' ' '); tmux display-message \"demo-it next failed: $__msg\"; fi",
		shellQuote(debugLogPath),
		shellQuote(cliPath),
	)
	if err := runTmux(
		"bind-key", "-T", "demo-it-nav", "n",
		"run-shell", "-b", nextCommand,
		"\\;", "switch-client", "-T", "root",
	); err != nil {
		return fmt.Errorf("bind demo-it next key: %w", err)
	}

	prevCommand := fmt.Sprintf(
		"__out=$(DEMO_IT_DEBUG_LOG=%s %s prev 2>&1); __code=$?; if [ \"$__code\" -ne 0 ]; then __msg=$(printf '%%s' \"$__out\" | tr '\\n' ' '); tmux display-message \"demo-it prev failed: $__msg\"; fi",
		shellQuote(debugLogPath),
		shellQuote(cliPath),
	)
	if err := runTmux(
		"bind-key", "-T", "demo-it-nav", "p",
		"run-shell", "-b", prevCommand,
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

func refreshNotesSession(notesSession string, workspacePath string, notesText string) error {
	if err := runTmux("respawn-pane", "-k", "-t", notesSession, "-c", workspacePath, "sh", "-lc", notesPaneCommand(notesText)); err != nil {
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

func notesPaneCommand(notesText string) string {
	payload := base64.StdEncoding.EncodeToString([]byte(notesText))
	return fmt.Sprintf(
		"printf '%%s' %s | base64 -d | nvim - '+setlocal buftype=nofile bufhidden=hide noswapfile nobuflisted' '+silent keepalt file [demo-it-notes]' '+silent! DemoItPresentationEnable'",
		shellQuote(payload),
	)
}

type workspaceStepTransition struct {
	Available       bool
	StepIndex       int
	TotalSteps      int
	StepTitle       string
	ActionsExecuted bool
}

func advanceWorkspaceStepForCommand(command string, runID string, socketPath string) (workspaceStepTransition, error) {
	debugf("advance workspace command=%q", command)
	sessions, err := listManagedSessionDetails()
	if err != nil {
		return workspaceStepTransition{}, err
	}

	demoSession, notesSession, ok := selectWorkspaceSessions(sessions)
	if !ok {
		debugf("advance workspace no managed sessions")
		if err := syncAutoNextWithDaemon(nil, -1, runID, socketPath); err != nil {
			return workspaceStepTransition{}, err
		}
		return workspaceStepTransition{}, nil
	}
	debugf("advance workspace selected demo=%q notes_present=%v step=%d", demoSession.Name, notesSession != nil, demoSession.Step)

	transcriptPath := strings.TrimSpace(demoSession.TranscriptPath)
	if transcriptPath == "" {
		transcriptPath = filepath.Join(demoSession.Workspace, "demo-it.md")
	}
	stepsPath := transcriptPath
	steps, err := transcript.ParseStepsFile(stepsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if syncErr := syncAutoNextWithDaemon(nil, -1, runID, socketPath); syncErr != nil {
				return workspaceStepTransition{}, syncErr
			}
			return workspaceStepTransition{}, nil
		}
		return workspaceStepTransition{}, fmt.Errorf("parse %s: %w", stepsPath, err)
	}
	if len(steps) == 0 {
		if err := syncAutoNextWithDaemon(nil, -1, runID, socketPath); err != nil {
			return workspaceStepTransition{}, err
		}
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
		if err := syncAutoNextWithDaemon(steps, demoSession.Step, runID, socketPath); err != nil {
			return workspaceStepTransition{}, err
		}
		notesText := renderSpeakerNotes(steps, demoSession.Step)
		if err := syncNotesWithDaemon(notesText, runID, socketPath); err != nil {
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
	if err := setSessionMetadata(demoSession.Name, demoSession.Workspace, transcriptPath, "demo", nextStep); err != nil {
		return workspaceStepTransition{}, err
	}
	notesText := renderSpeakerNotes(steps, nextStep)
	if err := syncNotesWithDaemon(notesText, runID, socketPath); err != nil {
		return workspaceStepTransition{}, err
	}
	if notesSession != nil {
		if err := setSessionMetadata(notesSession.Name, demoSession.Workspace, transcriptPath, "notes", nextStep); err != nil {
			return workspaceStepTransition{}, err
		}
		if err := refreshNotesSession(notesSession.Name, demoSession.Workspace, notesText); err != nil {
			return workspaceStepTransition{}, err
		}
	}
	if err := syncAutoNextWithDaemon(steps, nextStep, runID, socketPath); err != nil {
		return workspaceStepTransition{}, err
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

func syncNotesWithDaemon(notesText string, runID string, socketPath string) error {
	supported, err := daemonSupportsCapability(runID, socketPath, protocol.CapabilitySetNotes)
	if err != nil {
		return err
	}
	if !supported {
		return fmt.Errorf("connected daemon does not support %q; restart demo-itd with a newer build", protocol.CapabilitySetNotes)
	}
	raw, err := json.Marshal(protocol.SetNotesArgs{Text: notesText})
	if err != nil {
		return fmt.Errorf("encode set_notes args: %w", err)
	}
	req := protocol.Request{
		ID:      fmt.Sprintf("cli-set-notes-%d", time.Now().UnixNano()),
		Command: protocol.CommandSetNotes,
		RunID:   runID,
		Args:    raw,
	}
	c := client.SocketClient{SocketPath: socketPath}
	resp, err := c.Send(req)
	if err != nil {
		return fmt.Errorf("set notes via daemon: %w", wrapLocalDaemonUnavailable(err, socketPath))
	}
	if !resp.OK {
		if resp.Error == nil {
			return errors.New("daemon returned unknown error for set_notes")
		}
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	return nil
}

func syncAutoNextWithDaemon(steps []transcript.Step, stepIndex int, runID string, socketPath string) error {
	args, err := autoNextArgsForStep(steps, stepIndex, socketPath)
	if err != nil {
		return err
	}

	supported, err := daemonSupportsCapability(runID, socketPath, protocol.CapabilitySetAutoNext)
	if err != nil {
		return err
	}
	if !supported {
		if args.Enabled {
			return fmt.Errorf("connected daemon does not support %q; restart demo-itd with a newer build", protocol.CapabilitySetAutoNext)
		}
		debugf("skip set_auto_next disable run_id=%q (daemon missing capability)", runID)
		return nil
	}

	raw, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("encode set_auto_next args: %w", err)
	}

	req := protocol.Request{
		ID:      fmt.Sprintf("cli-auto-next-%d", time.Now().UnixNano()),
		Command: protocol.CommandSetAutoNext,
		RunID:   runID,
		Args:    raw,
	}
	debugf("set_auto_next run_id=%q enabled=%v delay_ms=%d socket=%q", runID, args.Enabled, args.DelayMS, socketPath)
	c := client.SocketClient{SocketPath: socketPath}
	resp, err := c.Send(req)
	if err != nil {
		return fmt.Errorf("set auto-next via daemon: %w", wrapLocalDaemonUnavailable(err, socketPath))
	}
	if !resp.OK {
		if resp.Error == nil {
			return errors.New("daemon returned unknown error for set_auto_next")
		}
		debugf("set_auto_next failed code=%q message=%q", resp.Error.Code, resp.Error.Message)
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	debugf("set_auto_next ok run_id=%q enabled=%v delay_ms=%d", runID, args.Enabled, args.DelayMS)
	return nil
}

func stepAutoSlideDelayMS(steps []transcript.Step, stepIndex int) (int, bool) {
	if stepIndex < 0 || stepIndex >= len(steps) {
		return 0, false
	}
	if stepIndex+1 >= len(steps) {
		return 0, false
	}
	autoSlideInMS := steps[stepIndex].AutoSlideInMS
	if autoSlideInMS == nil || *autoSlideInMS <= 0 {
		return 0, false
	}
	return *autoSlideInMS, true
}

func resolveCLIPathForAutoNext() (string, error) {
	override := strings.TrimSpace(os.Getenv("DEMO_IT_PATH"))
	if override != "" {
		return resolveCLIPath()
	}
	executablePath, err := os.Executable()
	if err == nil && strings.TrimSpace(executablePath) != "" {
		if resolved, lookErr := exec.LookPath(executablePath); lookErr == nil {
			return resolved, nil
		}
	}
	return resolveCLIPath()
}

func autoNextArgsForStep(steps []transcript.Step, stepIndex int, socketPath string) (protocol.SetAutoNextArgs, error) {
	delayMS, ok := stepAutoSlideDelayMS(steps, stepIndex)
	if !ok {
		return protocol.SetAutoNextArgs{Enabled: false}, nil
	}
	cliPath, err := resolveCLIPathForAutoNext()
	if err != nil {
		return protocol.SetAutoNextArgs{}, err
	}
	return protocol.SetAutoNextArgs{
		Enabled:      true,
		DelayMS:      delayMS,
		CLIPath:      cliPath,
		SocketPath:   socketPath,
		DebugLogPath: strings.TrimSpace(os.Getenv("DEMO_IT_DEBUG_LOG")),
		Env:          os.Environ(),
	}, nil
}

func daemonSupportsCapability(runID string, socketPath string, capability string) (bool, error) {
	c := client.SocketClient{SocketPath: socketPath}
	statusReq := protocol.Request{
		ID:      fmt.Sprintf("cli-capabilities-%d", time.Now().UnixNano()),
		Command: protocol.CommandStatus,
		RunID:   runID,
	}

	resp, err := c.Send(statusReq)
	if err != nil {
		return false, fmt.Errorf("check daemon capabilities: %w", wrapLocalDaemonUnavailable(err, socketPath))
	}
	if !resp.OK && resp.Error != nil && resp.Error.Code == "run_not_found" {
		if err := ensureRunStarted(runID, socketPath); err != nil {
			return false, err
		}
		resp, err = c.Send(statusReq)
		if err != nil {
			return false, fmt.Errorf("check daemon capabilities: %w", wrapLocalDaemonUnavailable(err, socketPath))
		}
	}
	if !resp.OK {
		if resp.Error == nil {
			return false, errors.New("daemon returned unknown error while checking capabilities")
		}
		return false, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	return stateSupportsCapability(resp.State, capability), nil
}

func stateSupportsCapability(state interface{}, capability string) bool {
	stateMap, ok := state.(map[string]interface{})
	if !ok {
		return false
	}
	raw, ok := stateMap["capabilities"]
	if !ok {
		return false
	}
	values, ok := raw.([]interface{})
	if !ok {
		return false
	}
	for _, value := range values {
		capabilityName, ok := value.(string)
		if !ok {
			continue
		}
		if capabilityName == capability {
			return true
		}
	}
	return false
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

func statusCommand(runID string, socketPath string) error {
	workspaces, err := listManagedWorkspaces()
	if err != nil {
		return err
	}
	if len(workspaces) == 0 {
		fmt.Println("no demo-it tmux sessions")
		return nil
	}

	active := workspaces[0]
	status := map[string]any{
		"workspace":     active.Workspace,
		"demo_session":  active.DemoSession,
		"notes_session": active.NotesSession,
	}
	if daemonState, ok := daemonRunStatusState(runID, socketPath); ok {
		status["run"] = daemonState
	}

	bytes, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("format status response: %w", err)
	}
	fmt.Println(string(bytes))
	return nil
}

func daemonRunStatusState(runID string, socketPath string) (map[string]any, bool) {
	req := protocol.Request{
		ID:      fmt.Sprintf("cli-status-%d", time.Now().UnixNano()),
		Command: protocol.CommandStatus,
		RunID:   runID,
	}
	c := client.SocketClient{SocketPath: socketPath}
	resp, err := c.Send(req)
	if err != nil || !resp.OK {
		return nil, false
	}
	bytes, err := json.Marshal(resp.State)
	if err != nil {
		return nil, false
	}
	state := map[string]any{}
	if err := json.Unmarshal(bytes, &state); err != nil {
		return nil, false
	}
	return state, true
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
	workspace, ok := latestWorkspaceByRole(workspaces, role)
	if !ok {
		return ""
	}
	switch role {
	case "demo":
		return workspace.DemoSession
	case "notes":
		return workspace.NotesSession
	default:
		return ""
	}
}

func latestWorkspaceByRole(workspaces []managedWorkspace, role string) (managedWorkspace, bool) {
	for _, workspace := range workspaces {
		switch role {
		case "demo":
			if workspace.DemoSession != "" {
				return workspace, true
			}
		case "notes":
			if workspace.NotesSession != "" {
				return workspace, true
			}
		}
	}
	return managedWorkspace{}, false
}

func traceNextCommand(runID string, socketPath string) (err error) {
	workspaces, err := listManagedWorkspaces()
	if err != nil {
		return err
	}
	if len(workspaces) == 0 {
		return errors.New("no demo-it tmux sessions")
	}

	workspace, ok := latestWorkspaceByRole(workspaces, "demo")
	if !ok {
		return errors.New("no demo-it demo session found")
	}
	if strings.TrimSpace(workspace.Workspace) == "" {
		return errors.New("latest demo workspace path is unavailable; restart with 'demo-it start'")
	}

	paneID, err := activePaneID(workspace.DemoSession)
	if err != nil {
		return err
	}

	logPath := traceLogPath(workspace.Workspace, "next", paneID, time.Now())
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create trace directory: %w", err)
	}
	header := fmt.Sprintf("# demo-it trace-next\n# session: %s\n# pane: %s\n# timestamp: %s\n\n", workspace.DemoSession, paneID, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(logPath, []byte(header), 0o644); err != nil {
		return fmt.Errorf("create trace log: %w", err)
	}

	pipeCommand := fmt.Sprintf("cat >> %s", shellQuote(logPath))
	if err := runTmux("pipe-pane", "-t", paneID, pipeCommand); err != nil {
		return fmt.Errorf("start pane trace for %s: %w", paneID, err)
	}
	started := true
	defer func() {
		if !started {
			return
		}
		stopErr := runTmux("pipe-pane", "-t", paneID)
		if stopErr != nil && err == nil {
			err = fmt.Errorf("stop pane trace for %s: %w", paneID, stopErr)
		}
	}()

	fmt.Fprintf(os.Stderr, "tracing pane %s -> %s\n", paneID, logPath)
	if err := runProtocolCommand("next", nil, runID, socketPath); err != nil {
		return err
	}
	if err := runTmux("pipe-pane", "-t", paneID); err != nil {
		return fmt.Errorf("stop pane trace for %s: %w", paneID, err)
	}
	started = false

	textPath, err := writeTraceTextSnapshot(logPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "trace saved: %s\n", logPath)
	fmt.Fprintf(os.Stderr, "trace text: %s\n", textPath)
	return nil
}

func activePaneID(sessionName string) (string, error) {
	output, err := runTmuxOutput("list-panes", "-t", sessionName, "-F", "#{pane_id}\t#{pane_active}")
	if err != nil {
		return "", fmt.Errorf("list panes for %s: %w", sessionName, err)
	}

	paneID, err := parseActivePaneIDOutput(output)
	if err != nil {
		return "", fmt.Errorf("resolve active pane for %s: %w", sessionName, err)
	}
	return paneID, nil
}

func parseActivePaneIDOutput(output string) (string, error) {
	firstPane := ""
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		paneID := strings.TrimSpace(parts[0])
		active := strings.TrimSpace(parts[1])
		if paneID == "" {
			continue
		}
		if firstPane == "" {
			firstPane = paneID
		}
		if active == "1" {
			return paneID, nil
		}
	}
	if firstPane != "" {
		return firstPane, nil
	}
	return "", errors.New("no panes found")
}

func traceLogPath(workspacePath string, action string, paneID string, now time.Time) string {
	paneLabel := strings.TrimPrefix(strings.TrimSpace(paneID), "%")
	if paneLabel == "" {
		paneLabel = "unknown"
	}
	fileName := fmt.Sprintf("%s-%s-pane-%s.log", now.Format("20060102-150405"), action, paneLabel)
	return filepath.Join(workspacePath, ".demo-it", "traces", fileName)
}

func traceTextSnapshotPath(rawPath string) string {
	if strings.HasSuffix(rawPath, ".log") {
		return strings.TrimSuffix(rawPath, ".log") + ".txt"
	}
	return rawPath + ".txt"
}

func writeTraceTextSnapshot(rawLogPath string) (string, error) {
	raw, err := os.ReadFile(rawLogPath)
	if err != nil {
		return "", fmt.Errorf("read trace log: %w", err)
	}
	normalized := normalizeTraceForTextSnapshot(string(raw))
	textPath := traceTextSnapshotPath(rawLogPath)
	if err := os.WriteFile(textPath, []byte(normalized), 0o644); err != nil {
		return "", fmt.Errorf("write trace text snapshot: %w", err)
	}
	return textPath, nil
}

func normalizeTraceForTextSnapshot(raw string) string {
	cleaned := strings.ReplaceAll(raw, "\r\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")
	cleaned = traceAnsiOSC.ReplaceAllString(cleaned, "")
	cleaned = traceAnsiDCS.ReplaceAllString(cleaned, "")
	cleaned = traceAnsiCSI.ReplaceAllString(cleaned, "")
	cleaned = traceAnsiSingle.ReplaceAllString(cleaned, "")
	cleaned = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r >= 32 && r != 127 {
			return r
		}
		return -1
	}, cleaned)

	lines := strings.Split(cleaned, "\n")
	out := make([]string, 0, len(lines))
	prevBlank := false
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t")
		if strings.HasPrefix(trimmedRight, "# demo-it trace-next") || strings.HasPrefix(trimmedRight, "# session:") || strings.HasPrefix(trimmedRight, "# pane:") || strings.HasPrefix(trimmedRight, "# timestamp:") {
			continue
		}
		if strings.TrimSpace(trimmedRight) == "" {
			if prevBlank || len(out) == 0 {
				continue
			}
			out = append(out, "")
			prevBlank = true
			continue
		}
		out = append(out, trimmedRight)
		prevBlank = false
	}

	joined := strings.TrimSpace(strings.Join(out, "\n"))
	if joined == "" {
		return ""
	}
	return joined + "\n"
}

type recordEvent struct {
	Timestamp time.Time `json:"ts"`
	Kind      string    `json:"kind"`
}

func recordCommand(rawArgs []string) error {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	title := fs.String("title", "Recorded interaction", "title for the generated demo-it block")
	if err := fs.Parse(rawArgs); err != nil {
		return err
	}

	workspacePath, err := resolveRecordWorkspacePath(fs.Args())
	if err != nil {
		return err
	}

	recordSession := runctx.SessionPrefix(workspacePath) + "-record"
	exists, err := tmuxSessionExists(recordSession)
	if err != nil {
		return err
	}
	if exists {
		if err := runTmux("kill-session", "-t", recordSession); err != nil {
			return fmt.Errorf("reset record session %q: %w", recordSession, err)
		}
	}

	now := time.Now()
	recordDir := filepath.Join(workspacePath, ".demo-it", "recordings", now.Format("20060102-150405"))
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return fmt.Errorf("create record directory: %w", err)
	}
	eventsPath := filepath.Join(recordDir, "events.ndjson")
	if err := os.WriteFile(eventsPath, nil, 0o644); err != nil {
		return fmt.Errorf("create record events file: %w", err)
	}
	inputDir := filepath.Join(recordDir, "inputs")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		return fmt.Errorf("create record inputs directory: %w", err)
	}

	cliPath, err := resolveCLIPathForAutoNext()
	if err != nil {
		return err
	}

	scriptPath, err := exec.LookPath("script")
	if err != nil {
		return fmt.Errorf("record requires 'script' utility in PATH: %w", err)
	}

	if err := createRecordSession(recordSession, workspacePath, cliPath, scriptPath, eventsPath, inputDir); err != nil {
		_ = runTmux("kill-session", "-t", recordSession)
		return err
	}
	defer func() {
		_ = runTmux("kill-session", "-t", recordSession)
	}()

	fmt.Fprintf(os.Stderr, "recording in tmux session %q\n", recordSession)
	fmt.Fprintln(os.Stderr, "exit the recording shell (Ctrl-D) to finish and print the demo-it block")

	if os.Getenv("TMUX") != "" {
		if err := runTmux("switch-client", "-t", recordSession); err != nil && !isTmuxNoCurrentClientError(err) {
			return err
		}
		if err := waitForSessionEnd(recordSession, 8*time.Hour); err != nil {
			return err
		}
	} else {
		if err := runTmuxWithStdio("attach-session", "-t", recordSession); err != nil {
			return err
		}
		exists, err := tmuxSessionExists(recordSession)
		if err != nil {
			return err
		}
		if exists {
			if err := runTmux("kill-session", "-t", recordSession); err != nil {
				return fmt.Errorf("stop record session %q: %w", recordSession, err)
			}
		}
	}

	events, err := readRecordEvents(eventsPath)
	if err != nil {
		return err
	}
	semanticActions := recordedTimedActions(events)
	inputActions, err := recordedInputTimedActions(inputDir)
	if err != nil {
		return err
	}
	actions := actionsFromTimed(mergeRecordedTimedActions(semanticActions, inputActions))
	if len(actions) == 0 {
		fmt.Fprintln(os.Stderr, "no pane-management or key actions were detected; emitting placeholder action block")
	}
	block, err := renderRecordedBlock(strings.TrimSpace(*title), actions)
	if err != nil {
		return err
	}
	fmt.Println(block)
	return nil
}

func resolveRecordWorkspacePath(rawArgs []string) (string, error) {
	workspacePath := ""
	switch len(rawArgs) {
	case 0:
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve current working directory: %w", err)
		}
		workspacePath = cwd
	case 1:
		workspacePath = rawArgs[0]
	default:
		return "", fmt.Errorf("record accepts at most one workspace path")
	}

	absolutePath, err := filepath.Abs(workspacePath)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	stat, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("workspace path: %w", err)
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("workspace path is not a directory: %s", absolutePath)
	}
	return absolutePath, nil
}

func createRecordSession(sessionName string, workspacePath string, cliPath string, scriptPath string, eventsPath string, inputDir string) error {
	if err := runTmux("new-session", "-d", "-s", sessionName, "-c", workspacePath); err != nil {
		return fmt.Errorf("create record tmux session %q: %w", sessionName, err)
	}
	if err := runTmux("set-option", "-t", sessionName, "-q", "@demo_it_record", "1"); err != nil {
		return fmt.Errorf("mark record tmux session %q: %w", sessionName, err)
	}
	if err := runTmux("set-option", "-t", sessionName, "-q", "status", "off"); err != nil {
		return fmt.Errorf("hide tmux status for %q: %w", sessionName, err)
	}
	if err := runTmux("set-environment", "-t", sessionName, "DEMO_IT_RECORD_EVENTS", eventsPath); err != nil {
		return fmt.Errorf("set record events env for %q: %w", sessionName, err)
	}
	if err := runTmux("set-environment", "-t", sessionName, "DEMO_IT_RECORD_INPUT_DIR", inputDir); err != nil {
		return fmt.Errorf("set record input dir env for %q: %w", sessionName, err)
	}
	if err := runTmux("set-environment", "-t", sessionName, "DEMO_IT_RECORD_SCRIPT_PATH", scriptPath); err != nil {
		return fmt.Errorf("set record script path env for %q: %w", sessionName, err)
	}
	defaultCommand := fmt.Sprintf("%s __record-shell", shellQuote(cliPath))
	if err := runTmux("set-option", "-t", sessionName, "-q", "default-command", defaultCommand); err != nil {
		return fmt.Errorf("set record default command for %q: %w", sessionName, err)
	}
	if err := runTmux("respawn-pane", "-k", "-t", sessionName, cliPath, "__record-shell"); err != nil {
		return fmt.Errorf("start record shell for %q: %w", sessionName, err)
	}

	splitPaneHookCmd := fmt.Sprintf("run-shell %s", shellQuote(recordSplitEventShellCommand(cliPath, eventsPath)))
	if err := runTmux("set-hook", "-t", sessionName, "after-split-window", splitPaneHookCmd); err != nil {
		return fmt.Errorf("set record split-pane hook: %w", err)
	}
	killPaneHookCmd := fmt.Sprintf("run-shell %s", shellQuote(recordEventShellCommand(cliPath, eventsPath, "kill-pane")))
	if err := runTmux("set-hook", "-t", sessionName, "after-kill-pane", killPaneHookCmd); err != nil {
		return fmt.Errorf("set record kill-pane hook: %w", err)
	}
	return nil
}

func recordEventShellCommand(cliPath string, eventsPath string, kind string) string {
	return fmt.Sprintf("%s __record-event --events %s --kind %s", shellQuote(cliPath), shellQuote(eventsPath), shellQuote(kind))
}

func recordSplitEventShellCommand(cliPath string, eventsPath string) string {
	return fmt.Sprintf("%s __record-split-event --events %s --hook-args \"#{hook_arguments}\"", shellQuote(cliPath), shellQuote(eventsPath))
}

func waitForSessionEnd(sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		exists, err := tmuxSessionExists(sessionName)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("recording timed out waiting for tmux session %q to end", sessionName)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func recordEventCommand(rawArgs []string) error {
	fs := flag.NewFlagSet("__record-event", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	eventsPath := fs.String("events", "", "events log path")
	kind := fs.String("kind", "", "recorded event kind")
	if err := fs.Parse(rawArgs); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("__record-event does not accept positional arguments")
	}

	resolvedPath, err := resolveRecordEventsPath(strings.TrimSpace(*eventsPath), "__record-event")
	if err != nil {
		return err
	}
	resolvedKind := strings.TrimSpace(*kind)
	if !isRecordEventKind(resolvedKind) {
		return fmt.Errorf("unsupported record event kind %q", resolvedKind)
	}
	return appendRecordEvent(resolvedPath, resolvedKind)
}

func recordSplitEventCommand(rawArgs []string) error {
	fs := flag.NewFlagSet("__record-split-event", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	eventsPath := fs.String("events", "", "events log path")
	hookArgs := fs.String("hook-args", "", "tmux hook arguments")
	if err := fs.Parse(rawArgs); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("__record-split-event does not accept positional arguments")
	}

	resolvedPath, err := resolveRecordEventsPath(strings.TrimSpace(*eventsPath), "__record-split-event")
	if err != nil {
		return err
	}
	kind := recordSplitKindFromHookArguments(strings.TrimSpace(*hookArgs))
	return appendRecordEvent(resolvedPath, kind)
}

func recordShellCommand(rawArgs []string) error {
	fs := flag.NewFlagSet("__record-shell", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	paneID := fs.String("pane-id", "", "tmux pane id")
	paneIndex := fs.String("pane-index", "", "tmux pane index")
	if err := fs.Parse(rawArgs); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("__record-shell does not accept positional arguments")
	}

	inputDir := strings.TrimSpace(os.Getenv("DEMO_IT_RECORD_INPUT_DIR"))
	if inputDir == "" {
		return errors.New("missing DEMO_IT_RECORD_INPUT_DIR for __record-shell")
	}
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		return fmt.Errorf("create input dir for __record-shell: %w", err)
	}

	resolvedPaneID := strings.TrimSpace(*paneID)
	if resolvedPaneID == "" {
		resolvedPaneID = strings.TrimSpace(os.Getenv("TMUX_PANE"))
	}
	resolvedPaneIndex := strings.TrimSpace(*paneIndex)
	if resolvedPaneIndex == "" {
		resolvedPaneIndex = "unknown"
	}

	paneLabel := recordPaneLogLabel(resolvedPaneID, resolvedPaneIndex)
	inputPath := filepath.Join(inputDir, paneLabel+".in.log")
	timingPath := filepath.Join(inputDir, paneLabel+".timing.log")
	startPath := filepath.Join(inputDir, paneLabel+".start")
	if err := os.WriteFile(startPath, []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o644); err != nil {
		return fmt.Errorf("write record shell start time: %w", err)
	}

	shellPath := strings.TrimSpace(os.Getenv("SHELL"))
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	scriptBin := strings.TrimSpace(os.Getenv("DEMO_IT_RECORD_SCRIPT_PATH"))
	if scriptBin == "" {
		resolvedScript, err := exec.LookPath("script")
		if err != nil {
			return fmt.Errorf("resolve script for __record-shell: %w", err)
		}
		scriptBin = resolvedScript
	}
	scriptCommand := shellPath + " -i"
	cmd := exec.Command(scriptBin, "-q", "-f", "-e", "--log-in", inputPath, "--log-out", "/dev/null", "--log-timing", timingPath, "--command", scriptCommand)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("record shell command: %w", err)
	}
	return nil
}

func recordPaneLogLabel(paneID string, paneIndex string) string {
	indexLabel := sanitizeRecordLogPart(paneIndex)
	if indexLabel == "" {
		indexLabel = "unknown"
	}
	idLabel := sanitizeRecordLogPart(strings.TrimPrefix(paneID, "%"))
	if idLabel == "" {
		idLabel = "unknown"
	}
	return fmt.Sprintf("pane-%s-%s", indexLabel, idLabel)
}

func sanitizeRecordLogPart(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	builder := strings.Builder{}
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func resolveRecordEventsPath(pathValue string, commandName string) (string, error) {
	resolvedPath := strings.TrimSpace(pathValue)
	if resolvedPath == "" {
		resolvedPath = strings.TrimSpace(os.Getenv("DEMO_IT_RECORD_EVENTS"))
	}
	if resolvedPath == "" {
		return "", fmt.Errorf("missing --events for %s", commandName)
	}
	return resolvedPath, nil
}

func appendRecordEvent(eventsPath string, kind string) error {
	event := recordEvent{Timestamp: time.Now().UTC(), Kind: kind}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode record event: %w", err)
	}

	file, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open record events log %q: %w", eventsPath, err)
	}
	defer file.Close()
	if _, err := file.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("append record event: %w", err)
	}
	return nil
}

func recordSplitKindFromHookArguments(hookArgs string) string {
	withPad := " " + strings.TrimSpace(hookArgs) + " "
	if strings.Contains(withPad, " -v ") {
		return "split-pane-vertical"
	}
	if strings.Contains(withPad, " -h ") {
		return "split-pane"
	}
	return "split-pane"
}

func isRecordEventKind(kind string) bool {
	switch kind {
	case "split-pane", "split-pane-vertical", "kill-pane", "killall-pane":
		return true
	default:
		return false
	}
}

func readRecordEvents(eventsPath string) ([]recordEvent, error) {
	file, err := os.Open(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("read record events %q: %w", eventsPath, err)
	}
	defer file.Close()

	events := make([]recordEvent, 0)
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var event recordEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, fmt.Errorf("decode record event at line %d: %w", line, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan record events: %w", err)
	}
	return events, nil
}

type timedRecordedAction struct {
	Timestamp time.Time
	Sequence  int
	Action    transcript.Action
}

type recordInputChunk struct {
	Timestamp time.Time
	Length    int
}

type recordedKeyEvent struct {
	Timestamp time.Time
	Key       string
}

func recordedTimedActions(events []recordEvent) []timedRecordedAction {
	actions := make([]timedRecordedAction, 0)
	sequence := 0
	for i := 0; i < len(events); i++ {
		event := events[i]
		switch event.Kind {
		case "split-pane":
			actions = append(actions, timedRecordedAction{Timestamp: event.Timestamp, Sequence: sequence, Action: transcript.Action{Kind: "split-pane"}})
			sequence++
		case "split-pane-vertical":
			actions = append(actions, timedRecordedAction{Timestamp: event.Timestamp, Sequence: sequence, Action: transcript.Action{Kind: "split-pane-vertical"}})
			sequence++
		case "killall-pane":
			actions = append(actions, timedRecordedAction{Timestamp: event.Timestamp, Sequence: sequence, Action: transcript.Action{Kind: "killall-pane"}})
			sequence++
		case "kill-pane":
			count := 1
			for i+1 < len(events) && events[i+1].Kind == "kill-pane" {
				count++
				i++
			}
			kind := "kill-pane"
			if count > 1 {
				kind = "killall-pane"
			}
			actions = append(actions, timedRecordedAction{Timestamp: event.Timestamp, Sequence: sequence, Action: transcript.Action{Kind: kind}})
			sequence++
		}
	}
	return actions
}

func recordedActions(events []recordEvent) []transcript.Action {
	return actionsFromTimed(recordedTimedActions(events))
}

func recordedInputTimedActions(inputDir string) ([]timedRecordedAction, error) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read record input dir %q: %w", inputDir, err)
	}

	actions := make([]timedRecordedAction, 0)
	sequence := 100000
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".timing.log") {
			continue
		}

		timingPath := filepath.Join(inputDir, name)
		inputPath := filepath.Join(inputDir, strings.TrimSuffix(name, ".timing.log")+".in.log")
		paneSelector := recordPaneSelectorFromLogName(name)
		chunkActions, err := recordedInputActionsForPane(inputPath, timingPath, paneSelector)
		if err != nil {
			return nil, err
		}
		for i := range chunkActions {
			chunkActions[i].Sequence = sequence
			sequence++
			actions = append(actions, chunkActions[i])
		}
	}
	return actions, nil
}

func recordedInputActionsForPane(inputPath string, timingPath string, paneSelector string) ([]timedRecordedAction, error) {
	startOverride, err := readRecordedInputStartOverride(timingPath)
	if err != nil {
		return nil, err
	}
	chunks, totalLength, err := parseRecordInputTiming(timingPath, startOverride)
	if err != nil {
		return nil, err
	}
	if totalLength == 0 {
		return nil, nil
	}

	rawInput, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read record input log %q: %w", inputPath, err)
	}
	payload, err := extractRecordInputPayload(rawInput, totalLength)
	if err != nil {
		return nil, fmt.Errorf("extract input payload from %q: %w", inputPath, err)
	}

	keyEvents := make([]recordedKeyEvent, 0)
	offset := 0
	for _, chunk := range chunks {
		if chunk.Length <= 0 {
			continue
		}
		if offset+chunk.Length > len(payload) {
			return nil, fmt.Errorf("input payload is shorter than timing chunk lengths in %q", timingPath)
		}
		chunkData := payload[offset : offset+chunk.Length]
		offset += chunk.Length
		keyEvents = append(keyEvents, recordedKeyEventsFromInputChunk(chunkData, chunk.Timestamp)...)
	}
	return recordTimedActionsFromKeyEvents(keyEvents, paneSelector), nil
}

func readRecordedInputStartOverride(timingPath string) (*time.Time, error) {
	startPath := strings.TrimSuffix(timingPath, ".timing.log") + ".start"
	bytes, err := os.ReadFile(startPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read record start time %q: %w", startPath, err)
	}
	value := strings.TrimSpace(string(bytes))
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, fmt.Errorf("parse record start time %q: %w", startPath, err)
	}
	return &parsed, nil
}

func parseRecordInputTiming(timingPath string, startOverride *time.Time) ([]recordInputChunk, int, error) {
	file, err := os.Open(timingPath)
	if err != nil {
		return nil, 0, fmt.Errorf("read record timing log %q: %w", timingPath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	startTime := time.Time{}
	if startOverride != nil {
		startTime = (*startOverride).UTC()
	}
	elapsed := time.Duration(0)
	chunks := make([]recordInputChunk, 0)
	totalLength := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		recordType := parts[0]
		if startTime.IsZero() && recordType == "H" && len(parts) >= 5 && parts[2] == "START_TIME" {
			value := parts[3] + " " + parts[4]
			parsed, err := time.Parse("2006-01-02 15:04:05-07:00", value)
			if err == nil {
				startTime = parsed
			}
		}

		deltaSeconds, err := strconv.ParseFloat(parts[1], 64)
		if err == nil {
			elapsed += time.Duration(deltaSeconds * float64(time.Second))
		}

		if recordType != "I" || startTime.IsZero() || len(parts) < 3 {
			continue
		}
		length, err := strconv.Atoi(parts[2])
		if err != nil || length <= 0 {
			continue
		}
		chunks = append(chunks, recordInputChunk{Timestamp: startTime.Add(elapsed), Length: length})
		totalLength += length
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("scan timing log %q: %w", timingPath, err)
	}
	if startTime.IsZero() {
		return nil, 0, fmt.Errorf("timing log %q is missing START_TIME", timingPath)
	}
	return chunks, totalLength, nil
}

func extractRecordInputPayload(rawInput []byte, totalLength int) ([]byte, error) {
	if totalLength <= 0 {
		return nil, nil
	}
	lineEnd := bytes.IndexByte(rawInput, '\n')
	if lineEnd < 0 {
		return nil, errors.New("missing script header line")
	}
	start := lineEnd + 1
	end := start + totalLength
	if end > len(rawInput) {
		return nil, errors.New("script input log is shorter than declared timing length")
	}
	payload := make([]byte, totalLength)
	copy(payload, rawInput[start:end])
	return payload, nil
}

func recordActionsFromInputChunk(data []byte, timestamp time.Time, paneSelector string) []timedRecordedAction {
	events := recordedKeyEventsFromInputChunk(data, timestamp)
	return recordTimedActionsFromKeyEvents(events, paneSelector)
}

func recordedKeyEventsFromInputChunk(data []byte, timestamp time.Time) []recordedKeyEvent {
	events := make([]recordedKeyEvent, 0, len(data))
	appendKey := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		events = append(events, recordedKeyEvent{Timestamp: timestamp, Key: trimmed})
	}

	for i := 0; i < len(data); {
		value := data[i]
		if value == '\r' || value == '\n' {
			appendKey("Enter")
			if value == '\r' && i+1 < len(data) && data[i+1] == '\n' {
				i += 2
				continue
			}
			i++
			continue
		}
		if value == '\t' {
			appendKey("Tab")
			i++
			continue
		}
		if value == 0x7f || value == 0x08 {
			appendKey("BSpace")
			i++
			continue
		}
		if value == 0x1b {
			if i+2 < len(data) && data[i+1] == '[' {
				switch data[i+2] {
				case 'A':
					appendKey("Up")
					i += 3
					continue
				case 'B':
					appendKey("Down")
					i += 3
					continue
				case 'C':
					appendKey("Right")
					i += 3
					continue
				case 'D':
					appendKey("Left")
					i += 3
					continue
				}
			}
			appendKey("Escape")
			i++
			continue
		}
		if value < 32 {
			if value >= 1 && value <= 26 {
				appendKey("C-" + string(rune('a'-1+value)))
			}
			i++
			continue
		}
		if value == ' ' {
			appendKey("Space")
			i++
			continue
		}
		appendKey(string([]byte{value}))
		i++
	}
	return events
}

func recordTimedActionsFromKeyEvents(events []recordedKeyEvent, paneSelector string) []timedRecordedAction {
	if len(events) == 0 {
		return nil
	}

	actions := make([]timedRecordedAction, 0)
	current := make([]recordedKeyEvent, 0)
	flush := func() {
		if len(current) == 0 {
			return
		}
		startDelay := 500
		action := transcript.Action{Kind: "key-macro", DelayMS: &startDelay, Keys: macroKeysFromEvents(current)}
		if paneSelector != "" {
			action.Pane = paneSelector
		}
		actions = append(actions, timedRecordedAction{Timestamp: current[0].Timestamp, Action: action})
		current = current[:0]
	}

	for _, event := range events {
		current = append(current, event)
		if event.Key == "Enter" {
			flush()
		}
	}
	flush()
	return actions
}

func macroKeysFromEvents(events []recordedKeyEvent) []transcript.KeyMacroKey {
	keys := make([]transcript.KeyMacroKey, 0, len(events))
	for i, event := range events {
		macroKey := transcript.KeyMacroKey{Key: event.Key}
		if i+1 < len(events) {
			delta := events[i+1].Timestamp.Sub(event.Timestamp)
			if delta > 0 {
				delayMS := int(delta / time.Millisecond)
				if delayMS > 0 {
					delay := delayMS
					macroKey.DelayMS = &delay
				}
			}
		}
		keys = append(keys, macroKey)
	}
	return keys
}

func recordPaneSelectorFromLogName(name string) string {
	trimmed := strings.TrimSuffix(name, ".timing.log")
	if !strings.HasPrefix(trimmed, "pane-") {
		return ""
	}
	parts := strings.Split(trimmed, "-")
	if len(parts) < 3 {
		return ""
	}
	indexValue := parts[1]
	if indexValue == "0" {
		return ""
	}
	if _, err := strconv.Atoi(indexValue); err != nil {
		return ""
	}
	return indexValue
}

func mergeRecordedTimedActions(left []timedRecordedAction, right []timedRecordedAction) []timedRecordedAction {
	merged := make([]timedRecordedAction, 0, len(left)+len(right))
	merged = append(merged, left...)
	merged = append(merged, right...)
	sort.SliceStable(merged, func(i, j int) bool {
		leftTime := merged[i].Timestamp
		rightTime := merged[j].Timestamp
		if leftTime.IsZero() && rightTime.IsZero() {
			return merged[i].Sequence < merged[j].Sequence
		}
		if leftTime.IsZero() {
			return false
		}
		if rightTime.IsZero() {
			return true
		}
		if leftTime.Equal(rightTime) {
			return merged[i].Sequence < merged[j].Sequence
		}
		return leftTime.Before(rightTime)
	})
	return merged
}

func actionsFromTimed(timed []timedRecordedAction) []transcript.Action {
	actions := make([]transcript.Action, 0, len(timed))
	for _, item := range timed {
		actions = append(actions, item.Action)
	}
	return actions
}

func renderRecordedBlock(title string, actions []transcript.Action) (string, error) {
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		trimmedTitle = "Recorded interaction"
	}
	if len(actions) == 0 {
		actions = []transcript.Action{{Kind: "insert-text", Text: "# TODO: add recorded terminal actions"}}
	}

	step := transcript.Step{
		Title:        trimmedTitle,
		Actions:      actions,
		SpeakerNotes: "TODO: add speaker notes.",
	}
	payload, err := yaml.Marshal(step)
	if err != nil {
		return "", fmt.Errorf("encode recorded block: %w", err)
	}
	return "```demo-it\n" + strings.TrimSpace(string(payload)) + "\n```", nil
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

	killedCount, err := killManagedWorkspaceSessions(selected)
	if err != nil {
		return err
	}
	fmt.Printf("killed %d demo-it tmux session(s)\n", killedCount)
	return nil
}

func killManagedWorkspaceSessions(workspaces []managedWorkspace) (int, error) {
	sessionNames := managedWorkspaceSessionNames(workspaces)
	killedCount := 0
	for _, session := range sessionNames {
		if err := runTmux("kill-session", "-t", session); err != nil {
			return killedCount, fmt.Errorf("kill tmux session %q: %w", session, err)
		}
		killedCount++
	}
	return killedCount, nil
}

func managedWorkspaceSessionNames(workspaces []managedWorkspace) []string {
	names := make([]string, 0)
	seen := map[string]struct{}{}
	for _, workspace := range workspaces {
		for _, session := range workspace.SessionNames {
			if session == "" {
				continue
			}
			if _, ok := seen[session]; ok {
				continue
			}
			seen[session] = struct{}{}
			names = append(names, session)
		}
	}
	return names
}

func selectWorkspacesToKill(workspaces []managedWorkspace, rawArgs []string) ([]managedWorkspace, error) {
	if len(rawArgs) > 0 {
		return nil, errors.New("kill does not accept session indexes; use 'demo-it kill'")
	}
	return workspaces, nil
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
	output, err := runTmuxOutput("list-sessions", "-F", "#{session_name}\t#{@demo_it}\t#{@demo_it_role}\t#{@demo_it_workspace}\t#{@demo_it_transcript}\t#{@demo_it_step}\t#{session_last_attached}")
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

		parts := strings.SplitN(line, "\t", 7)
		name := strings.TrimSpace(parts[0])
		marker := ""
		role := ""
		workspace := ""
		transcriptPath := ""
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
			transcriptPath = strings.TrimSpace(parts[4])
		}
		if len(parts) > 5 {
			if parsed, err := strconv.Atoi(strings.TrimSpace(parts[5])); err == nil {
				step = parsed
			}
		}
		if len(parts) > 6 {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(parts[6]), 10, 64); err == nil {
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
			Name:           name,
			Role:           role,
			Workspace:      workspace,
			TranscriptPath: transcriptPath,
			Step:           step,
			LastAttached:   lastAttached,
		})
	}

	return sessions, nil
}

func isLegacyDemoSession(name string) bool {
	return strings.HasSuffix(name, "-demo") || strings.HasSuffix(name, "-notes")
}

func isTmuxNoServerError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "no server running") || strings.Contains(message, "failed to connect to server") || strings.Contains(message, "error connecting to")
}

func isTmuxNoCurrentClientError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "no current client")
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
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		debugf("tmux cmd=%v err=%v output=%q", args, err, trimmed)
		return fmt.Errorf("tmux %v: %w: %s", args, err, trimmed)
	}
	if trimmed != "" {
		debugf("tmux cmd=%v output=%q", args, trimmed)
	} else {
		debugf("tmux cmd=%v ok", args)
	}
	return nil
}

func runTmuxOutput(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		debugf("tmux output cmd=%v err=%v output=%q", args, err, trimmed)
		return "", fmt.Errorf("tmux %v: %w: %s", args, err, trimmed)
	}
	if trimmed != "" {
		debugf("tmux output cmd=%v output=%q", args, trimmed)
	} else {
		debugf("tmux output cmd=%v ok", args)
	}
	return string(output), nil
}

func runTmuxWithStdio(args ...string) error {
	debugf("tmux stdio cmd=%v", args)
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		debugf("tmux stdio cmd=%v err=%v", args, err)
		return fmt.Errorf("tmux %v: %w", args, err)
	}
	return nil
}

func isProtocolCommand(name string) bool {
	switch name {
	case "start", "run-status", "reload", "next", "prev", "rerun", "jump", "focus", "list":
		return true
	default:
		return false
	}
}

func parseSubcommand(name string, rawArgs []string) (protocol.Command, json.RawMessage, error) {
	switch name {
	case "start":
		return protocol.CommandStart, nil, nil
	case "status", "run-status":
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
  record [--title <text>] [workspace-path]
  status
  run-status
  reload
  next
  prev
  rerun
  jump --slide <id|index>
  focus --policy <present|return|none>
  notes
  show
  trace-next
  kill

Start behavior:
  start (without a path) bootstraps the current working directory.
  start <workspace-path> bootstraps that workspace (expects <workspace>/demo-it.md).
  start <transcript-path.md> bootstraps from that transcript file.

Record behavior:
  record (without a path) records tmux actions in the current working directory.
  record uses a <workspace>-record session and prints a demo-it block on stop.

Workspace mode:
  Passing a workspace path directly is equivalent to start <workspace-path>.
  Passing a transcript file path directly is equivalent to start <transcript-path.md>.

Session lifecycle:
  demo-it status             # show active managed workspace/session status
  demo-it run-status         # show daemon run state JSON
  demo-it notes              # open notes session for active workspace
  demo-it show               # open demo session for active workspace
  demo-it trace-next         # trace active demo pane output while running next
  demo-it record --title T   # open <workspace>-record and print recorded block when shell exits
  demo-it kill               # kill managed sessions
`
