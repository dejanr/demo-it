package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunServerStartsAndStopsOnContextCancel(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "demo-itd-test.sock")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- runServer(ctx, socketPath)
	}()

	waitForSocket(t, socketPath)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServer returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runServer shutdown")
	}

	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("expected socket to be removed, stat err=%v", err)
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
