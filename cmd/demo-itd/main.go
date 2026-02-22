package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dejanr/demo-it/internal/daemon"
	"github.com/dejanr/demo-it/internal/runctx"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	repoRoot, err := runctx.RepoRoot()
	if err != nil {
		return err
	}

	defaultSocketPath := runctx.DefaultSocketPath(repoRoot)
	socketPath := flag.String("socket", defaultSocketPath, "daemon unix socket path")
	flag.Parse()

	service := daemon.NewService()
	server := daemon.NewServer(*socketPath, service)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return server.Start(ctx)
}
