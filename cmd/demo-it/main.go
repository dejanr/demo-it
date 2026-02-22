package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/dejanr/demo-it/internal/client"
	"github.com/dejanr/demo-it/internal/protocol"
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

	defaultRunID := runctx.DefaultRunID(repoRoot)
	defaultSocketPath := runctx.DefaultSocketPath(repoRoot)

	runID := flag.String("run-id", defaultRunID, "run identifier")
	socketPath := flag.String("socket", defaultSocketPath, "daemon unix socket path")
	flag.Parse()

	if flag.NArg() == 0 {
		return usageError()
	}

	cmd, args, err := parseSubcommand(flag.Arg(0), flag.Args()[1:])
	if err != nil {
		return err
	}

	req := protocol.Request{
		ID:      fmt.Sprintf("cli-%d", time.Now().UnixNano()),
		Command: cmd,
		RunID:   *runID,
		Args:    args,
	}

	c := client.SocketClient{SocketPath: *socketPath}
	resp, err := c.Send(req)
	if err != nil {
		return err
	}

	if !resp.OK {
		if resp.Error == nil {
			return errors.New("daemon returned unknown error")
		}
		return fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	bytes, err := json.MarshalIndent(resp.State, "", "  ")
	if err != nil {
		return fmt.Errorf("format response: %w", err)
	}
	fmt.Println(string(bytes))

	return nil
}

func parseSubcommand(name string, rawArgs []string) (protocol.Command, json.RawMessage, error) {
	switch name {
	case "start":
		return protocol.CommandStart, nil, nil
	case "status":
		return protocol.CommandStatus, nil, nil
	case "reload":
		return protocol.CommandReload, nil, nil
	case "next":
		return parseNextArgs(rawArgs, protocol.CommandNext)
	case "prev":
		return protocol.CommandPrev, nil, nil
	case "rerun":
		return parseNextArgs(rawArgs, protocol.CommandRerun)
	case "jump":
		return parseJumpArgs(rawArgs)
	case "focus":
		return parseFocusArgs(rawArgs)
	default:
		return "", nil, fmt.Errorf("unknown subcommand %q\n%s", name, usage)
	}
}

func parseNextArgs(rawArgs []string, command protocol.Command) (protocol.Command, json.RawMessage, error) {
	fs := flag.NewFlagSet(string(command), flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	focus := fs.String("focus", "", "focus policy: present|return|none")
	force := fs.Bool("force", false, "force execution")
	if err := fs.Parse(rawArgs); err != nil {
		return "", nil, err
	}

	args := protocol.NextArgs{Focus: protocol.FocusPolicy(*focus), Force: *force}
	encoded, err := json.Marshal(args)
	if err != nil {
		return "", nil, fmt.Errorf("encode args: %w", err)
	}
	return command, encoded, nil
}

func parseJumpArgs(rawArgs []string) (protocol.Command, json.RawMessage, error) {
	fs := flag.NewFlagSet("jump", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	slide := fs.String("slide", "", "slide id or zero-based slide index")
	focus := fs.String("focus", "", "focus policy: present|return|none")
	if err := fs.Parse(rawArgs); err != nil {
		return "", nil, err
	}

	if *slide == "" {
		return "", nil, fmt.Errorf("jump requires --slide <id|index>")
	}

	args := protocol.JumpArgs{Focus: protocol.FocusPolicy(*focus)}
	if idx, err := strconv.Atoi(*slide); err == nil {
		args.SlideIndex = &idx
	} else {
		args.SlideID = *slide
	}

	encoded, err := json.Marshal(args)
	if err != nil {
		return "", nil, fmt.Errorf("encode args: %w", err)
	}
	return protocol.CommandJump, encoded, nil
}

func parseFocusArgs(rawArgs []string) (protocol.Command, json.RawMessage, error) {
	fs := flag.NewFlagSet("focus", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	policy := fs.String("policy", "", "focus policy: present|return|none")
	if err := fs.Parse(rawArgs); err != nil {
		return "", nil, err
	}
	if *policy == "" {
		return "", nil, fmt.Errorf("focus requires --policy <present|return|none>")
	}

	args := protocol.SetFocusPolicyArgs{Focus: protocol.FocusPolicy(*policy)}

	encoded, err := json.Marshal(args)

	if err != nil {
		return "", nil, fmt.Errorf("encode args: %w", err)
	}
	return protocol.CommandSetFocusPolicy, encoded, nil
}

func usageError() error {
	return fmt.Errorf("missing subcommand\n%s", usage)
}

const usage = `Usage:
  demo-it [--run-id <id>] [--socket <path>] <command> [flags]

Commands:
  start
  status
  reload
  next [--focus <present|return|none>] [--force]
  prev
  rerun [--focus <present|return|none>] [--force]
  jump --slide <id|index> [--focus <present|return|none>]
  focus --policy <present|return|none>
`
