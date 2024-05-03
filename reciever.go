package firetap

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
)

type Receiver struct {
	listener net.Listener
}

func startReceiver() (*Receiver, error) {
	if err := os.RemoveAll(sockPath); err != nil {
		return nil, fmt.Errorf("failed to remove socket file: %v", err)
	}
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}
	logger.Info("firetap server is listening", "path", sockPath)
	return &Receiver{
		listener: listener,
	}, nil
}

func (r *Receiver) Run(ctx context.Context, ch chan<- string) error {
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down server")
			return nil
		default:
		}
		conn, err := r.listener.Accept()
		if err != nil {
			logger.Warn("failed to accept", "error", err)
			continue
		}
		go r.handleConnection(conn, ch)
	}
}

func (s *Receiver) handleConnection(conn net.Conn, ch chan<- string) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()
		select {
		case ch <- msg:
		default:
			logger.Warn("overflow", "message", msg)
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Error("reading from client", "error", err)
	}
}
