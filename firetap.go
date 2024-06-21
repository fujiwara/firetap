package firetap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	extensions "github.com/fujiwara/lambda-extensions"
)

var (
	lambdaExtensionAPIEndpoint string
	lambdaTelemetryAPIEndpoint string
	lambdaExtensionName        string
	bufferSize                 = 10000
)

const (
	listenPort       = 8080
	antiRecursionEnv = "FIRETAP_WRAPPED"
)

func init() {
	lambdaExtensionName = filepath.Base(os.Args[0])
}

func Run(ctx context.Context, opt *Option) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	slog.InfoContext(ctx, "running firetap", "option", opt)

	rcv, err := NewReceiver(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to start receiver", "error", err)
		return err
	}

	ext := extensions.NewClient(lambdaExtensionName)
	ext.CallbackShutdown = func(ctx context.Context, ev *extensions.ShutdownEvent) error {
		slog.InfoContext(ctx, "shutdown event received", "deadline", ev.DeadlineMs)
		cancel()
		return nil
	}
	if err := ext.Register(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to register extension", "error", err)
		return err
	}
	sub := extensions.DefaultTelemetrySubscription
	sub.Destination.URI = fmt.Sprintf("http://sandbox.localdomain:%d", listenPort)
	if err := ext.SubscribeTelemetry(ctx, &sub); err != nil {
		slog.ErrorContext(ctx, "failed to subscribe telemetry", "error", err)
		return err
	}

	sender, err := NewSender(ctx, opt.StreamName, opt.DataStream)
	if err != nil {
		slog.ErrorContext(ctx, "failed to start sender", "error", err)
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := ext.Run(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to run extension client", "error", err)
		}
		slog.InfoContext(ctx, "extension client stopped")
	}()
	go func() {
		defer wg.Done()
		err := rcv.Run(ctx, sender)
		if err != nil {
			slog.ErrorContext(ctx, "failed to run receiver", "error", err)
		}
		slog.InfoContext(ctx, "receiver stopped")
	}()
	wg.Wait()
	slog.InfoContext(ctx, "firetap stopped")
	return nil
}
