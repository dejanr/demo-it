package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dejanr/demo-it/internal/client"
	"github.com/dejanr/demo-it/internal/protocol"
)

func TestServerHandlesRequestOverUnixSocket(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "demo-it.sock")

	service := NewService()
	server := NewServer(socketPath, service)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	waitForSocket(t, socketPath)

	c := client.SocketClient{SocketPath: socketPath, Timeout: 2 * time.Second}

	startResp, err := c.Send(protocol.Request{ID: "1", Command: protocol.CommandStart, RunID: "demo-it-test"})
	if err != nil {
		t.Fatalf("send start: %v", err)
	}
	if !startResp.OK {
		t.Fatalf("expected start OK, got %#v", startResp.Error)
	}

	statusResp, err := c.Send(protocol.Request{ID: "2", Command: protocol.CommandStatus, RunID: "demo-it-test"})
	if err != nil {
		t.Fatalf("send status: %v", err)
	}
	if !statusResp.OK {
		t.Fatalf("expected status OK, got %#v", statusResp.Error)
	}

	state := decodeSocketState(t, statusResp.State)
	if state.RunID != "demo-it-test" {
		t.Fatalf("unexpected run id: %s", state.RunID)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server exited with error: %v", err)
	}
}

func waitForSocket(t *testing.T, socketPath string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket not created: %s", socketPath)
}

func decodeSocketState(t *testing.T, raw any) StateView {
	t.Helper()

	bytes, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}

	var state StateView
	if err := json.Unmarshal(bytes, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	return state
}
