package firetap

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Songmu/wrapcommander"
)

var (
	lambdaAPIEndpoint   string
	lambdaExtensionName string
	bufferSize          = 10000
	logger              *slog.Logger
)

const (
	lambdaExtensionNameHeader       = "Lambda-Extension-Name"
	lambdaExtensionIdentifierHeader = "Lambda-Extension-Identifier"
	sockPath                        = "/tmp/firetap.sock"
	antiRecursionEnv                = "FIRETAP_WRAPPED"
)

func init() {
	lambdaAPIEndpoint = "http://" + os.Getenv("AWS_LAMBDA_RUNTIME_API") + "/2020-01-01/extension"
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

func Wrapper(ctx context.Context, handler string) error {
	if os.Getenv(antiRecursionEnv) != "" {
		panic("recursive execution detected!!!")
	}
	if !filepath.IsAbs(handler) {
		handler = filepath.Join(os.Getenv("LAMBDA_TASK_ROOT"), handler)
	}
	logger.Info("running child command", "command", handler)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to connect to receiver: %v", err)
	}
	defer conn.Close()
	cmd := exec.CommandContext(ctx, handler)
	cmd.Stdout = conn
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Cancel = func() error {
		logger.Info("sending SIGTERM to child command")
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 1500 * time.Millisecond
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, antiRecursionEnv+"=1")

	if err := cmd.Run(); err != nil {
		exitCode := wrapcommander.ResolveExitCode(err)
		logger.Error("child command failed", "error", err, "exit_code", exitCode)
		return err
	}
	return nil
}

func Run(ctx context.Context, opt *Option) error {
	logger.Info("running firetap", "option", opt)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ext := NewExtensionClient()
	if err := ext.Register(ctx); err != nil {
		logger.Error("failed to register extension", "error", err)
		return err
	}

	rcv, err := startReceiver()
	if err != nil {
		logger.Error("failed to start receiver", "error", err)
		return err
	}

	sn, err := startSender(ctx, opt.StreamName, opt.DataStream)
	if err != nil {
		logger.Error("failed to start sender", "error", err)
		return err
	}

	ch := make(chan []byte, bufferSize)

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
		err := rcv.Run(ctx, ch)
		if err != nil {
			logger.Error("failed to run receiver", "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		err := sn.Run(ctx, ch)
		if err != nil {
			logger.Error("failed to run sender", "error", err)
		}
	}()
	wg.Wait()
	return nil
}
