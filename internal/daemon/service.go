package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/dejanr/demo-it/internal/protocol"
	"github.com/dejanr/demo-it/internal/session"
)

type Service struct {
	mu   sync.Mutex
	runs map[string]*session.RunState
}

type StateView struct {
	RunID              string            `json:"run_id"`
	Status             session.RunStatus `json:"status"`
	CurrentSlide       int               `json:"current_slide"`
	TranscriptRevision string            `json:"transcript_revision,omitempty"`
	LastEvent          string            `json:"last_event,omitempty"`
	InteractionID      string            `json:"interaction_id,omitempty"`
	Skipped            bool              `json:"skipped,omitempty"`
	Completed          bool              `json:"completed,omitempty"`
}

func NewService() *Service {
	return &Service{
		runs: map[string]*session.RunState{},
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
		return okResponse(req.ID, stateView(*run, session.TransitionResult{}))
	case protocol.CommandStatus:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		return okResponse(req.ID, stateView(*run, session.TransitionResult{}))
	case protocol.CommandReload:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		result := run.Reload(run.Slides, run.TranscriptRevision)
		return okResponse(req.ID, stateView(*run, result))
	case protocol.CommandNext:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}

		result, err := run.Next()
		if err != nil && !errors.Is(err, session.ErrNoFurtherTransition) {
			return errorResponse(req.ID, err)
		}
		return okResponse(req.ID, stateView(*run, result))
	case protocol.CommandPrev:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		result, err := run.Prev()
		if err != nil {
			return errorResponse(req.ID, err)
		}
		return okResponse(req.ID, stateView(*run, result))
	case protocol.CommandRerun:
		run, ok := s.runs[req.RunID]
		if !ok {
			return notFoundResponse(req.ID, req.RunID)
		}
		result, err := run.Rerun()
		if err != nil {
			return errorResponse(req.ID, err)
		}
		return okResponse(req.ID, stateView(*run, result))
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
		return okResponse(req.ID, stateView(*run, result))
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
		return okResponse(req.ID, stateView(*run, session.TransitionResult{Event: "set_focus_policy"}))
	default:
		return errorResponse(req.ID, fmt.Errorf("%w: unsupported cmd %q", protocol.ErrInvalidCommand, req.Command))
	}
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

func stateView(run session.RunState, transition session.TransitionResult) StateView {
	return StateView{
		RunID:              run.RunID,
		Status:             run.Status,
		CurrentSlide:       run.Cursor.Slide,
		TranscriptRevision: run.TranscriptRevision,
		LastEvent:          transition.Event,
		InteractionID:      transition.InteractionID,
		Skipped:            transition.Skipped,
		Completed:          transition.Completed,
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
