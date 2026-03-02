package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/dejanr/demo-it/internal/protocol"
	"github.com/dejanr/demo-it/internal/session"
)

type autoNextTask struct {
	Token uint64
	Timer *time.Timer
}

type autoNextExecutor func(runID string, args protocol.SetAutoNextArgs) error

type Service struct {
	mu               sync.Mutex
	runs             map[string]*session.RunState
	notes            map[string]string
	autoNextTasks    map[string]autoNextTask
	autoNextCounters map[string]uint64
	executeAutoNext  autoNextExecutor
}

type StateView struct {
	RunID              string            `json:"run_id"`
	Status             session.RunStatus `json:"status"`
	CurrentSlide       int               `json:"current_slide"`
	TranscriptRevision string            `json:"transcript_revision,omitempty"`
	Capabilities       []string          `json:"capabilities,omitempty"`
	SpeakerNotes       string            `json:"speaker_notes,omitempty"`
	LastEvent          string            `json:"last_event,omitempty"`
	InteractionID      string            `json:"interaction_id,omitempty"`
	Skipped            bool              `json:"skipped,omitempty"`
	Completed          bool              `json:"completed,omitempty"`
}

func NewService() *Service {
	return newServiceWithAutoNextExecutor(runAutoNextViaCLI)
}

func newServiceWithAutoNextExecutor(executor autoNextExecutor) *Service {
	if executor == nil {
		executor = runAutoNextViaCLI
	}
	return &Service{
		runs:             map[string]*session.RunState{},
		notes:            map[string]string{},
		autoNextTasks:    map[string]autoNextTask{},
		autoNextCounters: map[string]uint64{},
		executeAutoNext:  executor,
	}
}

func (s *Service) Handle(req protocol.Request) protocol.Response {
	if err := req.Validate(); err != nil {
		return errorResponse(req.ID, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch req.Command {
	case protocol.CommandStart:
		run := s.ensureRun(req.RunID)
		return okResponse(req.ID, s.stateView(*run, session.TransitionResult{}))
	case protocol.CommandStatus:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		return okResponse(req.ID, s.stateView(*run, session.TransitionResult{}))
	case protocol.CommandReload:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		result := run.Reload(run.Slides, run.TranscriptRevision)
		return okResponse(req.ID, s.stateView(*run, result))
	case protocol.CommandNext:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}

		result, err := run.Next()
		if err != nil && !errors.Is(err, session.ErrNoFurtherTransition) {
			return errorResponse(req.ID, err)
		}
		return okResponse(req.ID, s.stateView(*run, result))
	case protocol.CommandPrev:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		result, err := run.Prev()
		if err != nil {
			return errorResponse(req.ID, err)
		}
		return okResponse(req.ID, s.stateView(*run, result))
	case protocol.CommandRerun:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		result, err := run.Rerun()
		if err != nil {
			return errorResponse(req.ID, err)
		}
		return okResponse(req.ID, s.stateView(*run, result))
	case protocol.CommandJump:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}

		var args protocol.JumpArgs
		if err := decodeArgs(req.Args, &args); err != nil {
			return errorResponse(req.ID, fmt.Errorf("%w: %v", protocol.ErrInvalidRequest, err))
		}

		index, err := slideIndex(run.Slides, args)
		if err != nil {
			return errorResponse(req.ID, err)
		}

		result, err := run.JumpToSlide(index)
		if err != nil {
			return errorResponse(req.ID, err)
		}
		return okResponse(req.ID, s.stateView(*run, result))
	case protocol.CommandSetFocusPolicy:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		var args protocol.SetFocusPolicyArgs
		if err := decodeArgs(req.Args, &args); err != nil {
			return errorResponse(req.ID, fmt.Errorf("%w: %v", protocol.ErrInvalidRequest, err))
		}
		if err := run.SetDefaultFocusPolicy(args.Focus); err != nil {
			return errorResponse(req.ID, err)
		}
		return okResponse(req.ID, s.stateView(*run, session.TransitionResult{Event: "set_focus_policy"}))
	case protocol.CommandSetAutoNext:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		var args protocol.SetAutoNextArgs
		if err := decodeArgs(req.Args, &args); err != nil {
			return errorResponse(req.ID, fmt.Errorf("%w: %v", protocol.ErrInvalidRequest, err))
		}
		if err := s.setAutoNextLocked(req.RunID, args); err != nil {
			return errorResponse(req.ID, err)
		}
		event := "auto_next_cleared"
		if args.Enabled {
			event = "auto_next_set"
		}
		return okResponse(req.ID, s.stateView(*run, session.TransitionResult{Event: event}))
	case protocol.CommandSetNotes:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		var args protocol.SetNotesArgs
		if err := decodeArgs(req.Args, &args); err != nil {
			return errorResponse(req.ID, fmt.Errorf("%w: %v", protocol.ErrInvalidRequest, err))
		}
		s.notes[req.RunID] = args.Text
		return okResponse(req.ID, s.stateView(*run, session.TransitionResult{Event: "set_notes"}))
	default:
		return errorResponse(req.ID, fmt.Errorf("%w: unsupported cmd %q", protocol.ErrInvalidCommand, req.Command))
	}
}

func (s *Service) setAutoNextLocked(runID string, args protocol.SetAutoNextArgs) error {
	if task, ok := s.autoNextTasks[runID]; ok {
		task.Timer.Stop()
		delete(s.autoNextTasks, runID)
	}
	if !args.Enabled {
		return nil
	}

	nextToken := s.autoNextCounters[runID] + 1
	s.autoNextCounters[runID] = nextToken
	timer := time.AfterFunc(time.Duration(args.DelayMS)*time.Millisecond, func() {
		s.fireAutoNext(runID, nextToken, args)
	})
	s.autoNextTasks[runID] = autoNextTask{Token: nextToken, Timer: timer}
	return nil
}

func (s *Service) fireAutoNext(runID string, token uint64, args protocol.SetAutoNextArgs) {
	s.mu.Lock()
	task, ok := s.autoNextTasks[runID]
	if !ok || task.Token != token {
		s.mu.Unlock()
		return
	}
	delete(s.autoNextTasks, runID)
	executor := s.executeAutoNext
	s.mu.Unlock()

	if err := executor(runID, args); err != nil {
		writeAutoNextDebugLog(args.DebugLogPath, fmt.Sprintf("auto-next failed run_id=%s err=%v", runID, err))
	}
}

func writeAutoNextDebugLog(path string, message string) {
	debugPath := strings.TrimSpace(path)
	if debugPath == "" {
		return
	}
	file, err := os.OpenFile(debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() {
		_ = file.Close()
	}()
	_, _ = fmt.Fprintf(file, "%s %s\n", time.Now().Format(time.RFC3339Nano), message)
}

func runAutoNextViaCLI(runID string, args protocol.SetAutoNextArgs) error {
	cliPath := strings.TrimSpace(args.CLIPath)
	socketPath := strings.TrimSpace(args.SocketPath)
	if cliPath == "" || socketPath == "" {
		return fmt.Errorf("auto-next requires cli_path and socket_path")
	}

	cmd := exec.Command(cliPath, "--run-id", runID, "--socket", socketPath, "next")
	env := append([]string{}, args.Env...)
	if len(env) == 0 {
		env = os.Environ()
	}
	if debugLogPath := strings.TrimSpace(args.DebugLogPath); debugLogPath != "" {
		env = setOrReplaceEnv(env, "DEMO_IT_DEBUG_LOG", debugLogPath)
	}
	cmd.Env = env
	return cmd.Run()
}

func setOrReplaceEnv(env []string, key string, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			out = append(out, prefix+value)
			replaced = true
			continue
		}
		out = append(out, item)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func (s *Service) ensureRun(runID string) *session.RunState {
	run, ok := s.runs[runID]
	if ok {
		return run
	}

	state := session.NewRunState(runID, seedSlides(), "bootstrap")
	s.runs[runID] = &state
	return &state
}

func seedSlides() []session.Slide {
	return []session.Slide{
		{
			ID:    "intro",
			Title: "Intro",
			Interactions: []session.Interaction{
				{ID: "create-tracker", IdempotencyKey: "create-tracker-v1", Hash: "hash-a"},
				{ID: "update-tracker", IdempotencyKey: "update-tracker-v1", Hash: "hash-b"},
			},
		},
		{
			ID:           "wrap-up",
			Title:        "Wrap Up",
			Interactions: []session.Interaction{{ID: "extract-skill", IdempotencyKey: "extract-skill-v1", Hash: "hash-c"}},
		},
	}
}

func slideIndex(slides []session.Slide, args protocol.JumpArgs) (int, error) {
	if args.SlideIndex != nil {
		return *args.SlideIndex, nil
	}

	for idx, slide := range slides {
		if slide.ID == args.SlideID {
			return idx, nil
		}
	}

	return 0, fmt.Errorf("slide not found: %s", args.SlideID)
}

func (s *Service) stateView(run session.RunState, transition session.TransitionResult) StateView {
	return StateView{
		RunID:              run.RunID,
		Status:             run.Status,
		CurrentSlide:       run.Cursor.Slide,
		TranscriptRevision: run.TranscriptRevision,
		Capabilities: []string{
			protocol.CapabilitySetAutoNext,
			protocol.CapabilitySetNotes,
			protocol.CapabilitySplitPanes,
			protocol.CapabilityKillPanes,
			protocol.CapabilityKeyMacro,
		},
		SpeakerNotes:  s.notes[run.RunID],
		LastEvent:     transition.Event,
		InteractionID: transition.InteractionID,
		Skipped:       transition.Skipped,
		Completed:     transition.Completed,
	}
}

func okResponse(id string, state StateView) protocol.Response {
	return protocol.Response{ID: id, OK: true, State: state}
}

func notFoundResponse(id string, runID string) protocol.Response {
	return protocol.Response{
		ID: id,
		OK: false,
		Error: &protocol.APIError{
			Code:    "run_not_found",
			Message: fmt.Sprintf("run not started: %s", runID),
		},
	}
}

func errorResponse(id string, err error) protocol.Response {
	code := "internal"
	switch {
	case errors.Is(err, protocol.ErrInvalidCommand):
		code = "invalid_command"
	case errors.Is(err, protocol.ErrInvalidRequest):
		code = "invalid_request"
	case errors.Is(err, session.ErrNoSlides),
		errors.Is(err, session.ErrNoCurrentStep),
		errors.Is(err, session.ErrNoPreviousState),
		errors.Is(err, session.ErrSlideOutOfRange),
		errors.Is(err, session.ErrNoFurtherTransition):
		code = "invalid_state"
	}

	return protocol.Response{
		ID: id,
		OK: false,
		Error: &protocol.APIError{
			Code:    code,
			Message: err.Error(),
		},
	}
}

func decodeArgs(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
