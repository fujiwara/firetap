package firetap

import (
	"bufio"
	"context"
	"fmt"
	"io"
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
	logger.Info("receiver is listening", "path", sockPath)
	return &Receiver{
		listener: listener,
	}, nil
}

func (r *Receiver) Run(ctx context.Context, ch chan<- []byte) error {
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down receiver")
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

func (s *Receiver) handleConnection(conn net.Conn, ch chan<- []byte) {
	defer conn.Close()
	scanner := bufio.NewReader(conn)
	for {
		line, err := scanner.ReadBytes('\n')
		if err == io.EOF {
			if len(line) == 0 {
				// EOF
				break
			}
		} else if err != nil {
			logger.Error("reading from client", "error", err)
			break
		}
		select {
		case ch <- line:
		default:
			logger.Warn("overflow", "message", string(line))
		}
	}
}
