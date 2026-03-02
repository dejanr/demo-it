package daemon

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

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
	if !containsCapability(state.Capabilities, protocol.CapabilitySetAutoNext) {
		t.Fatalf("missing capability %q in %#v", protocol.CapabilitySetAutoNext, state.Capabilities)
	}
	if !containsCapability(state.Capabilities, protocol.CapabilitySplitPanes) {
		t.Fatalf("missing capability %q in %#v", protocol.CapabilitySplitPanes, state.Capabilities)
	}
	if !containsCapability(state.Capabilities, protocol.CapabilityKillPanes) {
		t.Fatalf("missing capability %q in %#v", protocol.CapabilityKillPanes, state.Capabilities)
	}
	if !containsCapability(state.Capabilities, protocol.CapabilityKeyMacro) {
		t.Fatalf("missing capability %q in %#v", protocol.CapabilityKeyMacro, state.Capabilities)
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

func TestHandleSetAutoNextExecutesAfterDelay(t *testing.T) {
	calls := make(chan protocol.SetAutoNextArgs, 1)
	svc := newServiceWithAutoNextExecutor(func(_ string, args protocol.SetAutoNextArgs) error {
		calls <- args
		return nil
	})
	_ = svc.Handle(protocol.Request{ID: "1", Command: protocol.CommandStart, RunID: "demo-it-test"})

	args, err := json.Marshal(protocol.SetAutoNextArgs{Enabled: true, DelayMS: 50, CLIPath: "/tmp/demo-it", SocketPath: "/tmp/demo.sock"})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	resp := svc.Handle(protocol.Request{ID: "2", Command: protocol.CommandSetAutoNext, RunID: "demo-it-test", Args: args})
	if !resp.OK {
		t.Fatalf("expected set_auto_next OK, got %#v", resp.Error)
	}

	select {
	case got := <-calls:
		if !got.Enabled || got.DelayMS != 50 {
			t.Fatalf("unexpected args: %#v", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected auto-next executor to be called")
	}
}

func TestHandleSetAutoNextCancelStopsPendingTimer(t *testing.T) {
	var mu sync.Mutex
	count := 0
	svc := newServiceWithAutoNextExecutor(func(_ string, _ protocol.SetAutoNextArgs) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})
	_ = svc.Handle(protocol.Request{ID: "1", Command: protocol.CommandStart, RunID: "demo-it-test"})

	setArgs, err := json.Marshal(protocol.SetAutoNextArgs{Enabled: true, DelayMS: 120, CLIPath: "/tmp/demo-it", SocketPath: "/tmp/demo.sock"})
	if err != nil {
		t.Fatalf("marshal set args: %v", err)
	}
	resp := svc.Handle(protocol.Request{ID: "2", Command: protocol.CommandSetAutoNext, RunID: "demo-it-test", Args: setArgs})
	if !resp.OK {
		t.Fatalf("expected set_auto_next OK, got %#v", resp.Error)
	}

	clearArgs, err := json.Marshal(protocol.SetAutoNextArgs{Enabled: false})
	if err != nil {
		t.Fatalf("marshal clear args: %v", err)
	}
	resp = svc.Handle(protocol.Request{ID: "3", Command: protocol.CommandSetAutoNext, RunID: "demo-it-test", Args: clearArgs})
	if !resp.OK {
		t.Fatalf("expected clear auto-next OK, got %#v", resp.Error)
	}

	time.Sleep(250 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if count != 0 {
		t.Fatalf("expected no executor calls, got %d", count)
	}
}

func containsCapability(capabilities []string, capability string) bool {
	for _, current := range capabilities {
		if current == capability {
			return true
		}
	}
	return false
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
