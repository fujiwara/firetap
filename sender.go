package firetap

import (
	"context"
)

type LogSender struct{}

func startSender() (*LogSender, error) {
	return &LogSender{}, nil
}

func (s *LogSender) Run(ctx context.Context, ch <-chan string) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-ch:
			logger.Info("TODO:received message", "message", msg)
		}
	}
}
