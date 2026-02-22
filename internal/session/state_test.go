package session

import (
	"errors"
	"testing"
)

func TestNextExecutesInteractionAndUpdatesCursor(t *testing.T) {
	state := NewRunState("run-1", demoSlides(), "rev-1")

	result, err := state.Next(false)
	if err != nil {
		t.Fatalf("next failed: %v", err)
	}

	if result.Event != "interaction" {
		t.Fatalf("expected interaction event, got %s", result.Event)
	}
	if state.Cursor.Slide != 0 || state.Cursor.Interaction != 0 {
		t.Fatalf("unexpected cursor: %+v", state.Cursor)
	}

	entry, ok := state.ExecutionLedger["create-tracker-v1"]
	if !ok {
		t.Fatal("expected ledger entry for first interaction")
	}
	if entry.StepHash != "hash-a" {
		t.Fatalf("unexpected step hash: %s", entry.StepHash)
	}
}

func TestNextMovesToNextSlideWhenCurrentSlideExhausted(t *testing.T) {
	state := NewRunState("run-1", demoSlides(), "rev-1")

	if _, err := state.Next(false); err != nil {
		t.Fatalf("next failed: %v", err)
	}
	if _, err := state.Next(false); err != nil {
		t.Fatalf("next failed: %v", err)
	}

	result, err := state.Next(false)
	if err != nil {
		t.Fatalf("expected slide transition, got error: %v", err)
	}
	if result.Event != "slide" {
		t.Fatalf("expected slide event, got %s", result.Event)
	}
	if state.Cursor.Slide != 1 || state.Cursor.Interaction != -1 {
		t.Fatalf("unexpected cursor after slide transition: %+v", state.Cursor)
	}
}

func TestPrevRestoresPreviousCursor(t *testing.T) {
	state := NewRunState("run-1", demoSlides(), "rev-1")
	if _, err := state.Next(false); err != nil {
		t.Fatalf("next failed: %v", err)
	}
	if _, err := state.Next(false); err != nil {
		t.Fatalf("next failed: %v", err)
	}

	result, err := state.Prev()
	if err != nil {
		t.Fatalf("prev failed: %v", err)
	}
	if result.Event != "prev" {
		t.Fatalf("expected prev event, got %s", result.Event)
	}
	if state.Cursor.Slide != 0 || state.Cursor.Interaction != 0 {
		t.Fatalf("expected cursor restored to (0,0), got %+v", state.Cursor)
	}
}

func TestRerunRequiresCurrentInteraction(t *testing.T) {
	state := NewRunState("run-1", demoSlides(), "rev-1")

	if _, err := state.Rerun(); !errors.Is(err, ErrNoCurrentStep) {
		t.Fatalf("expected ErrNoCurrentStep, got %v", err)
	}
}

func TestIdempotencySkipsAlreadyDoneStep(t *testing.T) {
	state := NewRunState("run-1", demoSlides(), "rev-1")
	if _, err := state.Next(false); err != nil {
		t.Fatalf("next failed: %v", err)
	}
	if _, err := state.Prev(); err != nil {
		t.Fatalf("prev failed: %v", err)
	}

	result, err := state.Next(false)
	if err != nil {
		t.Fatalf("next failed: %v", err)
	}
	if !result.Skipped {
		t.Fatal("expected interaction to be skipped by idempotency ledger")
	}
}

func TestReloadInvalidatesLedgerOnHashChange(t *testing.T) {
	state := NewRunState("run-1", demoSlides(), "rev-1")
	if _, err := state.Next(false); err != nil {
		t.Fatalf("next failed: %v", err)
	}

	changed := demoSlides()
	changed[0].Interactions[0].Hash = "hash-changed"
	state.Reload(changed, "rev-2")

	if _, ok := state.ExecutionLedger["create-tracker-v1"]; ok {
		t.Fatal("expected ledger entry removed after hash change")
	}
}

func TestJumpToSlideRejectsInvalidIndex(t *testing.T) {
	state := NewRunState("run-1", demoSlides(), "rev-1")

	if _, err := state.JumpToSlide(99); !errors.Is(err, ErrSlideOutOfRange) {
		t.Fatalf("expected ErrSlideOutOfRange, got %v", err)
	}
}

func demoSlides() []Slide {
	return []Slide{
		{
			ID:    "intro",
			Title: "Intro",
			Interactions: []Interaction{
				{ID: "create-tracker", IdempotencyKey: "create-tracker-v1", Hash: "hash-a"},
				{ID: "update-tracker", IdempotencyKey: "update-tracker-v1", Hash: "hash-b"},
			},
		},
		{
			ID:    "wrap-up",
			Title: "Wrap Up",
			Interactions: []Interaction{
				{ID: "extract-skill", IdempotencyKey: "extract-skill-v1", Hash: "hash-c"},
			},
		},
	}
}
