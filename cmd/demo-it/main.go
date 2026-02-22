package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	}

	if !isProtocolCommand(target) {
		return bootstrapWorkspace(target, *runID, *socketPath)
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
	resp, err := c.Send(req)
	if err != nil {
		return err
	}

	if !resp.OK {
		if resp.Error == nil {
			return errors.New("daemon returned unknown error")
		}
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	bytes, err := json.MarshalIndent(resp.State, "", "  ")
	if err != nil {
		return fmt.Errorf("format response: %w", err)
	}
	fmt.Println(string(bytes))

	return nil
}

func bootstrapWorkspace(rawPath string, runID string, socketPath string) error {
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

	if err := executeBootstrapStep(workspacePath, demoSession); err != nil {
		return err
	}

	if err := ensureRunStarted(runID, socketPath); err != nil {
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
	return nil
}

func executeBootstrapStep(workspacePath string, demoSession string) error {
	stepsPath := filepath.Join(workspacePath, "demo-it.md")
	steps, err := transcript.ParseStepsFile(stepsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("parse %s: %w", stepsPath, err)
	}
	if len(steps) == 0 {
		return nil
	}
	return runActions(demoSession, steps[0].Actions)
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

func listSessionsCommand() error {
	sessions, err := listDemoItSessions()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("no demo-it tmux sessions")
		return nil
	}
	for idx, session := range sessions {
		fmt.Printf("%d\t%s\n", idx+1, session)
	}
	return nil
}

func killSessionsCommand(rawArgs []string) error {
	sessions, err := listDemoItSessions()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("no demo-it tmux sessions")
		return nil
	}

	selected, err := selectSessionsToKill(sessions, rawArgs)
	if err != nil {
		return err
	}

	for _, session := range selected {
		if err := runTmux("kill-session", "-t", session); err != nil {
			return fmt.Errorf("kill tmux session %q: %w", session, err)
		}
	}
	fmt.Printf("killed %d demo-it tmux session(s)\n", len(selected))
	return nil
}

func selectSessionsToKill(sessions []string, rawArgs []string) ([]string, error) {
	if len(rawArgs) == 0 {
		return sessions, nil
	}

	selected := make([]string, 0, len(rawArgs))
	seen := map[int]struct{}{}
	for _, arg := range rawArgs {
		idx, err := strconv.Atoi(arg)
		if err != nil {
			return nil, fmt.Errorf("kill expects numeric session indexes from 'demo-it list', got %q", arg)
		}
		if idx < 1 || idx > len(sessions) {
			return nil, fmt.Errorf("kill index out of range: %d (valid: 1..%d)", idx, len(sessions))
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		selected = append(selected, sessions[idx-1])
	}
	return selected, nil
}

func listDemoItSessions() ([]string, error) {
	output, err := runTmuxOutput("list-sessions", "-F", "#{session_name}\t#{@demo_it}")
	if err != nil {
		if isTmuxNoServerError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list tmux sessions: %w", err)
	}

	sessions := make([]string, 0)
	seen := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, "\t", 2)
		name := parts[0]
		marker := ""
		if len(parts) > 1 {
			marker = strings.TrimSpace(parts[1])
		}

		if marker != "1" && !isLegacyDemoSession(name) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		sessions = append(sessions, name)
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

	focus := fs.String("focus", "", "focus policy: present|return|none")
	force := fs.Bool("force", false, "force execution")
	if err := fs.Parse(rawArgs); err != nil {
		return "", nil, err
	}

	args := protocol.NextArgs{Focus: protocol.FocusPolicy(*focus), Force: *force}
	encoded, err := json.Marshal(args)
	if err != nil {
		return "", nil, fmt.Errorf("encode args: %w", err)
	}
	return command, encoded, nil
}

func parseJumpArgs(rawArgs []string) (protocol.Command, json.RawMessage, error) {
	fs := flag.NewFlagSet("jump", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	slide := fs.String("slide", "", "slide id or zero-based slide index")
	focus := fs.String("focus", "", "focus policy: present|return|none")
	if err := fs.Parse(rawArgs); err != nil {
		return "", nil, err
	}

	if *slide == "" {
		return "", nil, fmt.Errorf("jump requires --slide <id|index>")
	}

	args := protocol.JumpArgs{Focus: protocol.FocusPolicy(*focus)}
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
  start
  status
  reload
  next [--focus <present|return|none>] [--force]
  prev
  rerun [--focus <present|return|none>] [--force]
  jump --slide <id|index> [--focus <present|return|none>]
  focus --policy <present|return|none>
  list
  kill [session-index ...]

Workspace mode:
  Resets and starts <name>-demo and <name>-notes tmux sessions for the path,
  then opens/switches to <name>-demo.

Session lifecycle:
  demo-it list               # show numbered managed sessions
  demo-it kill               # kill all managed sessions
  demo-it kill 1 3           # kill selected session indexes from list
`
