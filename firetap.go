package firetap

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	lambdaExtensionAPIEndpoint string
	lambdaTelemetryAPIEndpoint string
	lambdaExtensionName        string
	bufferSize                 = 10000
)

const (
	lambdaExtensionNameHeader       = "Lambda-Extension-Name"
	lambdaExtensionIdentifierHeader = "Lambda-Extension-Identifier"
	listenPort                      = 8080
	antiRecursionEnv                = "FIRETAP_WRAPPED"
)

func init() {
	lambdaExtensionAPIEndpoint = "http://" + os.Getenv("AWS_LAMBDA_RUNTIME_API") + "/2020-01-01/extension"
	lambdaTelemetryAPIEndpoint = "http://" + os.Getenv("AWS_LAMBDA_RUNTIME_API") + "/2022-07-01/telemetry"
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

	ext := NewExtensionClient(ctx)
	if err := ext.Register(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to register extension", "error", err)
		return err
	}
	if err := ext.SubscribeTelemetry(ctx, rcv.Endpoint); err != nil {
		slog.ErrorContext(ctx, "failed to subscribe telemetry", "error", err)
		return err
	}

	sender, err := NewSender(ctx, opt.StreamName, opt.DataStream)
	if err != nil {
		slog.ErrorContext(ctx, "failed to start sender", "error", err)
		return err
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		err := ext.Run(ctx, cancel)
		if err != nil {
			slog.ErrorContext(ctx, "failed to run extension client", "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		err := rcv.Run(ctx, sender)
		if err != nil {
			slog.ErrorContext(ctx, "failed to run receiver", "error", err)
		}
	}()
	wg.Wait()
	return nil
}
