package client

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dejanr/demo-it/internal/daemon"
	"github.com/dejanr/demo-it/internal/protocol"
)

func TestSocketClientSendStart(t *testing.T) {
	socketPath, shutdown := startTestDaemonServer(t)
	defer shutdown()

	c := SocketClient{SocketPath: socketPath}
	resp, err := c.Send(protocol.Request{ID: "req-1", Command: protocol.CommandStart, RunID: "demo-it-test"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected OK response, got %#v", resp.Error)
	}
}

func TestSocketClientSendDialError(t *testing.T) {
	missingSocket := filepath.Join(t.TempDir(), "missing.sock")
	c := SocketClient{SocketPath: missingSocket, Timeout: 100 * time.Millisecond}

	_, err := c.Send(protocol.Request{ID: "req-1", Command: protocol.CommandStatus, RunID: "demo-it-test"})
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !strings.Contains(err.Error(), "dial daemon socket") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func startTestDaemonServer(t *testing.T) (string, func()) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "client-test.sock")
	service := daemon.NewService()
	server := daemon.NewServer(socketPath, service)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	waitForSocket(t, socketPath)

	shutdown := func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("daemon server exited with error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for daemon server shutdown")
		}
	}

	return socketPath, shutdown
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
