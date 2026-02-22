package daemon

import (
	"encoding/json"
	"testing"

	"github.com/dejanr/demo-it/internal/protocol"
)

func TestHandleStartCreatesRunAndStatusReturnsState(t *testing.T) {
	svc := NewService()

	startResp := svc.Handle(protocol.Request{ID: "1", Command: protocol.CommandStart, RunID: "demo-it-test"})
	if !startResp.OK {
		t.Fatalf("expected start OK, got %#v", startResp.Error)
	}

	statusResp := svc.Handle(protocol.Request{ID: "2", Command: protocol.CommandStatus, RunID: "demo-it-test"})
	if !statusResp.OK {
		t.Fatalf("expected status OK, got %#v", statusResp.Error)
	}

	state := decodeState(t, statusResp.State)
	if state.RunID != "demo-it-test" {
		t.Fatalf("unexpected run id: %s", state.RunID)
	}
	if state.CurrentSlide != 0 {
		t.Fatalf("unexpected slide index: %d", state.CurrentSlide)
	}
}

func TestHandleNextAdvancesInteraction(t *testing.T) {
	svc := NewService()
	_ = svc.Handle(protocol.Request{ID: "1", Command: protocol.CommandStart, RunID: "demo-it-test"})

	resp := svc.Handle(protocol.Request{ID: "2", Command: protocol.CommandNext, RunID: "demo-it-test"})
	if !resp.OK {
		t.Fatalf("expected next OK, got %#v", resp.Error)
	}

	state := decodeState(t, resp.State)
	if state.LastEvent != "interaction" {
		t.Fatalf("expected interaction event, got %s", state.LastEvent)
	}
	if state.CurrentSlide != 0 {
		t.Fatalf("expected slide index 0, got %d", state.CurrentSlide)
	}
}

func TestHandleJumpBySlideID(t *testing.T) {
	svc := NewService()
	_ = svc.Handle(protocol.Request{ID: "1", Command: protocol.CommandStart, RunID: "demo-it-test"})

	args, err := json.Marshal(protocol.JumpArgs{SlideID: "wrap-up"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	resp := svc.Handle(protocol.Request{ID: "2", Command: protocol.CommandJump, RunID: "demo-it-test", Args: args})
	if !resp.OK {
		t.Fatalf("expected jump OK, got %#v", resp.Error)
	}

	state := decodeState(t, resp.State)
	if state.CurrentSlide != 1 {
		t.Fatalf("expected slide index 1, got %d", state.CurrentSlide)
	}
}

func TestHandleFocusPolicy(t *testing.T) {
	svc := NewService()
	_ = svc.Handle(protocol.Request{ID: "1", Command: protocol.CommandStart, RunID: "demo-it-test"})

	args, err := json.Marshal(protocol.SetFocusPolicyArgs{Focus: protocol.FocusReturn})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	resp := svc.Handle(protocol.Request{ID: "2", Command: protocol.CommandSetFocusPolicy, RunID: "demo-it-test", Args: args})
	if !resp.OK {
		t.Fatalf("expected focus policy OK, got %#v", resp.Error)
	}

	state := decodeState(t, resp.State)
	if state.LastEvent != "set_focus_policy" {
		t.Fatalf("expected set_focus_policy event, got %s", state.LastEvent)
	}
}

func TestHandleStatusForUnknownRun(t *testing.T) {
	svc := NewService()

	resp := svc.Handle(protocol.Request{ID: "1", Command: protocol.CommandStatus, RunID: "missing"})
	if resp.OK {
		t.Fatal("expected status to fail for unknown run")
	}
	if resp.Error == nil || resp.Error.Code != "run_not_found" {
		t.Fatalf("unexpected error: %#v", resp.Error)
	}
}

func decodeState(t *testing.T, raw any) StateView {
	t.Helper()

	bytes, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}

	var out StateView
	if err := json.Unmarshal(bytes, &out); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	return out
}
