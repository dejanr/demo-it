package protocol

import (
	"encoding/json"
	"testing"
)

func TestRequestValidateAcceptsNextWithoutArgs(t *testing.T) {
	raw, err := json.Marshal(NextArgs{})
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

func TestRequestValidateIgnoresUnknownNextArgs(t *testing.T) {
	raw := json.RawMessage(`{"focus":"present","force":true}`)
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

func TestRequestValidateSetAutoNext(t *testing.T) {
	args, err := json.Marshal(SetAutoNextArgs{
		Enabled:    true,
		DelayMS:    500,
		CLIPath:    "/tmp/demo-it",
		SocketPath: "/tmp/demo.sock",
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	req := Request{
		ID:      "req-1",
		Command: CommandSetAutoNext,
		RunID:   "demo-it-test",
		Args:    args,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid set_auto_next request, got %v", err)
	}
}

func TestRequestValidateRejectsSetAutoNextMissingFields(t *testing.T) {
	args := json.RawMessage(`{"enabled":true,"delay_ms":0}`)
	req := Request{
		ID:      "req-1",
		Command: CommandSetAutoNext,
		RunID:   "demo-it-test",
		Args:    args,
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
