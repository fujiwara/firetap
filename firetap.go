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
	logger                     *slog.Logger
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
	logger = slog.New(slog.NewJSONHandler(os.Stdout, nil)) // default logger
}

func LogType(name string) {
	logger = logger.With("type", name)
}

func Fatal(err error) {
	logger.Error("fatal error", "error", err)
	os.Exit(1)
}

func Run(ctx context.Context, opt *Option) error {
	logger.Info("running firetap", "option", opt)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	rcv, err := startReceiver()
	if err != nil {
		logger.Error("failed to start receiver", "error", err)
		return err
	}

	ext := NewExtensionClient()
	if err := ext.Register(ctx); err != nil {
		logger.Error("failed to register extension", "error", err)
		return err
	}
	if err := ext.SubscribeTelemetry(ctx, rcv.Endpoint); err != nil {
		logger.Error("failed to subscribe telemetry", "error", err)
		return err
	}

	sender, err := NewSender(ctx, opt.StreamName, opt.DataStream)
	if err != nil {
		logger.Error("failed to start sender", "error", err)
		return err
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		err := ext.Run(ctx, cancel)
		if err != nil {
			logger.Error("failed to run extension", "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		err := rcv.Run(ctx, sender)
		if err != nil {
			logger.Error("failed to run receiver", "error", err)
		}
	}()
	wg.Wait()
	return nil
}
