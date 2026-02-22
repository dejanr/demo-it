package session

import (
	"errors"
	"fmt"
	"time"

	"github.com/dejanr/demo-it/internal/protocol"
)

type RunStatus string

const (
	RunStatusIdle      RunStatus = "idle"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

var (
	ErrNoSlides            = errors.New("no slides loaded")
	ErrNoPreviousState     = errors.New("no previous state")
	ErrNoCurrentStep       = errors.New("no current interaction")
	ErrSlideOutOfRange     = errors.New("slide index out of range")
	ErrNoFurtherTransition = errors.New("already at end of transcript")
)

type Interaction struct {
	ID             string
	IdempotencyKey string
	Hash           string
}

type Slide struct {
	ID           string
	Title        string
	Interactions []Interaction
}

type Cursor struct {
	Slide       int
	Interaction int
}

type HistoryEntry struct {
	At            time.Time
	Kind          string
	Before        Cursor
	After         Cursor
	InteractionID string
	Skipped       bool
}

type LedgerEntry struct {
	StepHash  string
	Status    string
	UpdatedAt time.Time
}

type RunState struct {
	RunID              string
	Status             RunStatus
	DefaultFocusPolicy protocol.FocusPolicy
	TranscriptRevision string
	Slides             []Slide
	Cursor             Cursor
	History            []HistoryEntry
	ExecutionLedger    map[string]LedgerEntry
}

type TransitionResult struct {
	Cursor        Cursor
	Event         string
	InteractionID string
	Skipped       bool
	Completed     bool
}

func NewRunState(runID string, slides []Slide, transcriptRevision string) RunState {
	status := RunStatusIdle
	if len(slides) > 0 {
		status = RunStatusRunning
	}

	return RunState{
		RunID:              runID,
		Status:             status,
		DefaultFocusPolicy: protocol.FocusPresent,
		TranscriptRevision: transcriptRevision,
		Slides:             slides,
		Cursor: Cursor{
			Slide:       0,
			Interaction: -1,
		},
		ExecutionLedger: map[string]LedgerEntry{},
	}
}

func (s *RunState) SetDefaultFocusPolicy(policy protocol.FocusPolicy) error {
	if !policy.Valid() {
		return fmt.Errorf("invalid focus policy: %s", policy)
	}
	s.DefaultFocusPolicy = policy
	return nil
}

func (s *RunState) Next() (TransitionResult, error) {
	if len(s.Slides) == 0 {
		return TransitionResult{}, ErrNoSlides
	}

	current := s.Slides[s.Cursor.Slide]
	nextInteraction := s.Cursor.Interaction + 1
	if nextInteraction < len(current.Interactions) {
		interaction := current.Interactions[nextInteraction]
		skipped := false
		if interaction.IdempotencyKey != "" {
			entry, ok := s.ExecutionLedger[interaction.IdempotencyKey]
			if ok && entry.Status == "done" && entry.StepHash == interaction.Hash {
				skipped = true
			}
		}

		if !skipped {
			s.recordExecution(interaction)
		}

		before := s.Cursor
		s.Cursor.Interaction = nextInteraction
		s.pushHistory("next", before, s.Cursor, interaction.ID, skipped)

		completed := s.isLastInteraction(s.Cursor)
		if completed {
			s.Status = RunStatusCompleted
		} else {
			s.Status = RunStatusRunning
		}

		return TransitionResult{
			Cursor:        s.Cursor,
			Event:         "interaction",
			InteractionID: interaction.ID,
			Skipped:       skipped,
			Completed:     completed,
		}, nil
	}

	if s.Cursor.Slide+1 < len(s.Slides) {
		before := s.Cursor
		s.Cursor.Slide++
		s.Cursor.Interaction = -1
		s.Status = RunStatusRunning
		s.pushHistory("next_slide", before, s.Cursor, "", false)
		return TransitionResult{Cursor: s.Cursor, Event: "slide"}, nil
	}

	s.Status = RunStatusCompleted
	return TransitionResult{Cursor: s.Cursor, Event: "end", Completed: true}, ErrNoFurtherTransition
}

func (s *RunState) Prev() (TransitionResult, error) {
	if len(s.History) == 0 {
		return TransitionResult{}, ErrNoPreviousState
	}

	last := s.History[len(s.History)-1]
	s.History = s.History[:len(s.History)-1]
	s.Cursor = last.Before
	s.Status = RunStatusRunning

	return TransitionResult{Cursor: s.Cursor, Event: "prev"}, nil
}

func (s *RunState) Rerun() (TransitionResult, error) {
	interaction, err := s.currentInteraction()
	if err != nil {
		return TransitionResult{}, err
	}

	s.recordExecution(interaction)
	before := s.Cursor
	s.pushHistory("rerun", before, s.Cursor, interaction.ID, false)
	s.Status = RunStatusRunning

	return TransitionResult{
		Cursor:        s.Cursor,
		Event:         "rerun",
		InteractionID: interaction.ID,
	}, nil
}

func (s *RunState) JumpToSlide(index int) (TransitionResult, error) {
	if index < 0 || index >= len(s.Slides) {
		return TransitionResult{}, ErrSlideOutOfRange
	}

	before := s.Cursor
	s.Cursor.Slide = index
	s.Cursor.Interaction = -1
	s.Status = RunStatusRunning
	s.pushHistory("jump", before, s.Cursor, "", false)

	return TransitionResult{Cursor: s.Cursor, Event: "jump"}, nil
}

func (s *RunState) Reload(slides []Slide, revision string) TransitionResult {
	s.Slides = slides
	s.TranscriptRevision = revision

	valid := make(map[string]string)
	for _, slide := range slides {
		for _, interaction := range slide.Interactions {
			if interaction.IdempotencyKey == "" {
				continue
			}
			valid[interaction.IdempotencyKey] = interaction.Hash
		}
	}

	for key, entry := range s.ExecutionLedger {
		hash, ok := valid[key]
		if !ok || hash != entry.StepHash {
			delete(s.ExecutionLedger, key)
		}
	}

	s.reconcileCursor()
	if len(slides) == 0 {
		s.Status = RunStatusIdle
	} else {
		s.Status = RunStatusRunning
	}

	return TransitionResult{Cursor: s.Cursor, Event: "reload"}
}

func (s *RunState) currentInteraction() (Interaction, error) {
	if len(s.Slides) == 0 {
		return Interaction{}, ErrNoSlides
	}
	if s.Cursor.Slide < 0 || s.Cursor.Slide >= len(s.Slides) {
		return Interaction{}, ErrSlideOutOfRange
	}
	if s.Cursor.Interaction < 0 {
		return Interaction{}, ErrNoCurrentStep
	}
	slide := s.Slides[s.Cursor.Slide]
	if s.Cursor.Interaction >= len(slide.Interactions) {
		return Interaction{}, ErrNoCurrentStep
	}
	return slide.Interactions[s.Cursor.Interaction], nil
}

func (s *RunState) recordExecution(interaction Interaction) {
	if interaction.IdempotencyKey == "" {
		return
	}
	s.ExecutionLedger[interaction.IdempotencyKey] = LedgerEntry{
		StepHash:  interaction.Hash,
		Status:    "done",
		UpdatedAt: time.Now(),
	}
}

func (s *RunState) pushHistory(kind string, before Cursor, after Cursor, interactionID string, skipped bool) {
	s.History = append(s.History, HistoryEntry{
		At:            time.Now(),
		Kind:          kind,
		Before:        before,
		After:         after,
		InteractionID: interactionID,
		Skipped:       skipped,
	})
}

func (s *RunState) isLastInteraction(cursor Cursor) bool {
	if cursor.Slide != len(s.Slides)-1 {
		return false
	}
	lastSlide := s.Slides[cursor.Slide]
	return cursor.Interaction == len(lastSlide.Interactions)-1
}

func (s *RunState) reconcileCursor() {
	if len(s.Slides) == 0 {
		s.Cursor = Cursor{Slide: 0, Interaction: -1}
		return
	}

	if s.Cursor.Slide < 0 {
		s.Cursor.Slide = 0
	}
	if s.Cursor.Slide >= len(s.Slides) {
		s.Cursor.Slide = len(s.Slides) - 1
	}

	maxInteraction := len(s.Slides[s.Cursor.Slide].Interactions) - 1
	if s.Cursor.Interaction > maxInteraction {
		s.Cursor.Interaction = maxInteraction
	}
	if s.Cursor.Interaction < -1 {
		s.Cursor.Interaction = -1
	}
}
