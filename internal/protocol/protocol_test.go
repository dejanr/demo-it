package protocol

import (
	"encoding/json"
	"testing"
)

func TestRequestValidateAcceptsNextWithFocus(t *testing.T) {
	raw, err := json.Marshal(NextArgs{Focus: FocusPresent})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	req := Request{
		ID:      "req-1",
		Command: CommandNext,
		RunID:   "demo-it-test",
		Args:    raw,
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}
}

func TestRequestValidateRejectsUnsupportedCommand(t *testing.T) {
	req := Request{
		ID:      "req-1",
		Command: Command("fly"),
		RunID:   "demo-it-test",
	}

	if err := req.Validate(); err == nil {
		t.Fatal("expected validation error for unsupported command")
	}
}

func TestRequestValidateRejectsInvalidFocus(t *testing.T) {
	raw, err := json.Marshal(NextArgs{Focus: FocusPolicy("teleport")})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	req := Request{
		ID:      "req-1",
		Command: CommandNext,
		RunID:   "demo-it-test",
		Args:    raw,
	}

	if err := req.Validate(); err == nil {
		t.Fatal("expected validation error for invalid focus policy")
	}
}

func TestRequestValidateRejectsJumpWithoutTarget(t *testing.T) {
	req := Request{
		ID:      "req-1",
		Command: CommandJump,
		RunID:   "demo-it-test",
	}

	if err := req.Validate(); err == nil {
		t.Fatal("expected validation error for jump without target")
	}
}

func TestRequestValidateRequiresRunID(t *testing.T) {
	req := Request{
		ID:      "req-1",
		Command: CommandStatus,
	}

	if err := req.Validate(); err == nil {
		t.Fatal("expected validation error for missing run_id")
	}
}
