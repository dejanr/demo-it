package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/dejanr/demo-it/internal/protocol"
)

type SocketClient struct {
	SocketPath string
	Timeout    time.Duration
}

func (c SocketClient) Send(req protocol.Request) (protocol.Response, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	conn, err := net.DialTimeout("unix", c.SocketPath, timeout)
	if err != nil {
		return protocol.Response{}, fmt.Errorf("dial daemon socket: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return protocol.Response{}, fmt.Errorf("encode request: %w", err)
	}

	decoder := json.NewDecoder(bufio.NewReader(conn))
	var resp protocol.Response
	if err := decoder.Decode(&resp); err != nil {
		return protocol.Response{}, fmt.Errorf("decode response: %w", err)
	}

	return resp, nil
}
