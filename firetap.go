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
	bufferSize          = 1000
	logger              *slog.Logger
)

const (
	lambdaExtensionNameHeader       = "Lambda-Extension-Name"
	lambdaExtensionIdentifierHeader = "Lambda-Extension-Identifier"
	sockPath                        = "/tmp/firetap.sock"
)

func init() {
	lambdaAPIEndpoint = "http://" + os.Getenv("AWS_LAMBDA_RUNTIME_API") + "/2020-01-01/extension"
	lambdaExtensionName = filepath.Base(os.Args[0])
	logger = &slog.Logger{}
}

func SetLogger(l *slog.Logger) {
	logger = l
}

func Wrapper(ctx context.Context, handler string) error {
	if !filepath.IsAbs(handler) {
		handler = filepath.Join(os.Getenv("LAMBDA_TASK_ROOT"), handler)
	}
	logger.Info("running child command", "firetap", "wrapper", "command", handler)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}
	defer conn.Close()
	cmd := exec.CommandContext(ctx, "./"+"handler")
	cmd.Stdout = conn
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Cancel = func() error {
		logger.Info("sending SIGTERM to child command")
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 1500 * time.Millisecond

	if err := cmd.Run(); err != nil {
		exitCode := wrapcommander.ResolveExitCode(err)
		logger.Error("child command failed", "error", err, "exit_code", exitCode)
		return err
	}
	return nil
}

func Run(ctx context.Context) error {
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
	sn, err := startSender()
	if err != nil {
		logger.Error("failed to start sender", "error", err)
		return err
	}

	ch := make(chan string, bufferSize)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		err := ext.Run(ctx)
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
