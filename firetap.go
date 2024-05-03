package firetap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Songmu/wrapcommander"
)

var (
	lambdaAPIEndpoint   string
	lambdaExtensionName string
	bufferSize          = 1000
)

const (
	lambdaExtensionNameHeader       = "Lambda-Extension-Name"
	lambdaExtensionIdentifierHeader = "Lambda-Extension-Identifier"
	sockPath                        = "/tmp/firetap.sock"
)

func init() {
	lambdaAPIEndpoint = "http://" + os.Getenv("AWS_LAMBDA_RUNTIME_API") + "/2020-01-01/extension"
	lambdaExtensionName = filepath.Base(os.Args[0])
}

type ExtensionClient struct {
	extensionId string
	client      *http.Client
}

func NewExtensionClient() *ExtensionClient {
	return &ExtensionClient{
		client: http.DefaultClient,
	}
}

func (c *ExtensionClient) Register(ctx context.Context) error {
	if os.Getenv("FIRETAP_SKIP_EXTENSION") != "" {
		slog.Info("skipping extension registration")
		return nil
	}
	registerURL := fmt.Sprintf("%s/register", lambdaAPIEndpoint)
	req, _ := http.NewRequestWithContext(ctx, "POST", registerURL, strings.NewReader(`{"events":["INVOKE","SHUTDOWN"]}`))
	req.Header.Set(lambdaExtensionNameHeader, lambdaExtensionName)
	slog.Info("registering extension", "url", registerURL, "name", lambdaExtensionName, "headers", req.Header)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register extension: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode register response: %v", err)
	}
	slog.Info("register status", "status", resp.Status, "response", result)

	c.extensionId = resp.Header.Get(lambdaExtensionIdentifierHeader)
	if c.extensionId == "" {
		return fmt.Errorf("extension identifier is empty: %d %v", resp.StatusCode, resp.Header)
	}
	slog.Info("extension registered", "extension_id", c.extensionId)
	return nil
}

func (c *ExtensionClient) Run(ctx context.Context) error {
	if os.Getenv("FIRETAP_SKIP_EXTENSION") != "" {
		slog.Info("skipping extension running")
		<-ctx.Done()
		return nil
	}
	eventURL := fmt.Sprintf("%s/event/next", lambdaAPIEndpoint)
	for {
		slog.Info("getting next event")
		req, _ := http.NewRequestWithContext(ctx, "GET", eventURL, nil)
		req.Header.Set(lambdaExtensionIdentifierHeader, c.extensionId)
		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to get next event: %v", err)
		}
		defer resp.Body.Close()

		var event map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
			return fmt.Errorf("failed to decode event: %v", err)
		}
		slog.Info("event received", "event", event)

		switch event["eventType"] {
		case "INVOKE":
			slog.Info("invoke event received")
		case "SHUTDOWN":
			slog.Info("shutdown event received")
			return nil
		}
	}
}

type LogServer struct {
	listener net.Listener
}

func startServer() (*LogServer, error) {
	if err := os.RemoveAll(sockPath); err != nil {
		return nil, fmt.Errorf("failed to remove socket file: %v", err)
	}
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}
	slog.Info(fmt.Sprintf("firetap server is listening on %s", sockPath))
	return &LogServer{
		listener: listener,
	}, nil
}

func (s *LogServer) Run(ctx context.Context, ch chan<- string) error {
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down server")
			return nil
		default:
		}
		conn, err := s.listener.Accept()
		if err != nil {
			slog.Warn("failed to accept", "error", err)
			continue
		}
		go s.handleConnection(conn, ch)
	}
}

func (s *LogServer) handleConnection(conn net.Conn, ch chan<- string) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()
		select {
		case ch <- msg:
		default:
			slog.Warn("overflow message buffer", "message", msg)
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Error("reading from client", "error", err)
	}
}

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
			slog.Info("TODO:received message", "message", msg)
		}
	}
}

func Run(ctx context.Context) error {
	if os.Getenv("_HANDLER") != "" {
		// in runtime
		return Runtime(ctx)
	}
	// otherwise, in extension
	ext := NewExtensionClient()
	if err := ext.Register(ctx); err != nil {
		slog.Error("failed to register extension", "error", err)
		return err
	}

	sv, err := startServer()
	if err != nil {
		slog.Error("failed to start server", "error", err)
		return err
	}
	sn, err := startSender()
	if err != nil {
		slog.Error("failed to start sender", "error", err)
		return err
	}

	ch := make(chan string, bufferSize)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		err := ext.Run(ctx)
		if err != nil {
			slog.Error("failed to run extension", "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		err := sv.Run(ctx, ch)
		if err != nil {
			slog.Error("failed to run server", "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		err := sn.Run(ctx, ch)
		if err != nil {
			slog.Error("failed to run sender", "error", err)
		}
	}()
	wg.Wait()
	return nil
}

func Runtime(ctx context.Context) error {
	slog.Info("running child command", "firetap", "runtime", "command", "./handler")
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}
	defer conn.Close()
	cmd := exec.CommandContext(ctx, "./handler")
	cmd.Stdout = conn
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Cancel = func() error {
		slog.Info("sending SIGTERM to child command")
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 1500 * time.Millisecond

	if err := cmd.Run(); err != nil {
		exitCode := wrapcommander.ResolveExitCode(err)
		slog.Error("child command failed", "error", err, "exit_code", exitCode)
		return err
	}
	return nil
}
