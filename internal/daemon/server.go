package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dejanr/demo-it/internal/protocol"
)

type Server struct {
	socketPath string
	service    *Service

	listener net.Listener
	wg       sync.WaitGroup
}

func NewServer(socketPath string, service *Service) *Server {
	return &Server{socketPath: socketPath, service: service}
}

func (s *Server) Start(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	_ = os.Remove(s.socketPath)

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on socket: %w", err)
	}
	s.listener = listener

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			return fmt.Errorf("accept connection: %w", err)
		}

		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer func() {
				_ = c.Close()
			}()
			s.handleConn(c)
		}(conn)
	}

	s.wg.Wait()
	return nil
}

func (s *Server) Close() error {
	if s.listener == nil {
		return nil
	}

	err := s.listener.Close()
	_ = os.Remove(s.socketPath)
	return err
}

func (s *Server) handleConn(conn net.Conn) {
	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)

	for {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			return
		}

		var req protocol.Request
		if err := json.Unmarshal(line, &req); err != nil {
			resp := protocol.Response{
				ID: "",
				OK: false,
				Error: &protocol.APIError{
					Code:    "invalid_request",
					Message: fmt.Sprintf("decode request: %v", err),
				},
			}
			_ = encoder.Encode(resp)
			continue
		}

		resp := s.service.Handle(req)
		if err := encoder.Encode(resp); err != nil {
			return
		}
	}
}
