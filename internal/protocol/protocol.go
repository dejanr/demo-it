package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

type Command string

const (
	CommandStart          Command = "start"
	CommandStatus         Command = "status"
	CommandReload         Command = "reload"
	CommandNext           Command = "next"
	CommandPrev           Command = "prev"
	CommandRerun          Command = "rerun"
	CommandJump           Command = "jump"
	CommandSetFocusPolicy Command = "set_focus_policy"
	CommandSetAutoNext    Command = "set_auto_next"
)

const (
	CapabilitySetAutoNext = "set_auto_next"
	CapabilitySplitPanes  = "split_panes"
	CapabilityKillPanes   = "kill_panes"
	CapabilityKeyMacro    = "key_macro"
)

type FocusPolicy string

const (
	FocusPresent FocusPolicy = "present"
	FocusReturn  FocusPolicy = "return"
	FocusNone    FocusPolicy = "none"
)

type Request struct {
	ID      string          `json:"id"`
	Command Command         `json:"cmd"`
	RunID   string          `json:"run_id"`
	Args    json.RawMessage `json:"args,omitempty"`
}

type Response struct {
	ID    string      `json:"id"`
	OK    bool        `json:"ok"`
	State interface{} `json:"state,omitempty"`
	Error *APIError   `json:"error,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type NextArgs struct{}

type JumpArgs struct {
	SlideID    string `json:"slide_id,omitempty"`
	SlideIndex *int   `json:"slide_index,omitempty"`
}

type SetFocusPolicyArgs struct {
	Focus FocusPolicy `json:"focus"`
}

type SetAutoNextArgs struct {
	Enabled      bool     `json:"enabled"`
	DelayMS      int      `json:"delay_ms,omitempty"`
	CLIPath      string   `json:"cli_path,omitempty"`
	SocketPath   string   `json:"socket_path,omitempty"`
	DebugLogPath string   `json:"debug_log_path,omitempty"`
	Env          []string `json:"env,omitempty"`
}

var (
	ErrInvalidRequest = errors.New("invalid request")
	ErrInvalidCommand = errors.New("invalid command")
)

func (r Request) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidRequest)
	}
	if r.RunID == "" {
		return fmt.Errorf("%w: missing run_id", ErrInvalidRequest)
	}

	switch r.Command {
	case CommandStart, CommandStatus, CommandReload, CommandPrev:
		return nil
	case CommandNext, CommandRerun:
		var args NextArgs
		if err := decodeArgs(r.Args, &args); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		return nil
	case CommandJump:
		var args JumpArgs
		if err := decodeArgs(r.Args, &args); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		if args.SlideID == "" && args.SlideIndex == nil {
			return fmt.Errorf("%w: jump requires slide_id or slide_index", ErrInvalidRequest)
		}
		return nil
	case CommandSetFocusPolicy:
		var args SetFocusPolicyArgs
		if err := decodeArgs(r.Args, &args); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		if !args.Focus.Valid() {
			return fmt.Errorf("%w: invalid focus policy %q", ErrInvalidRequest, args.Focus)
		}
		return nil
	case CommandSetAutoNext:
		var args SetAutoNextArgs
		if err := decodeArgs(r.Args, &args); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}
		if !args.Enabled {
			return nil
		}
		if args.DelayMS <= 0 {
			return fmt.Errorf("%w: set_auto_next requires delay_ms > 0", ErrInvalidRequest)
		}
		if args.CLIPath == "" {
			return fmt.Errorf("%w: set_auto_next requires cli_path", ErrInvalidRequest)
		}
		if args.SocketPath == "" {
			return fmt.Errorf("%w: set_auto_next requires socket_path", ErrInvalidRequest)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported cmd %q", ErrInvalidCommand, r.Command)
	}
}

func (f FocusPolicy) Valid() bool {
	switch f {
	case FocusPresent, FocusReturn, FocusNone:
		return true
	default:
		return false
	}
}

func decodeArgs(raw json.RawMessage, out interface{}) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	return nil
}
