package firetap

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	slogcontext "github.com/PumpkinSeed/slog-context"
	"github.com/Songmu/wrapcommander"
	"github.com/samber/lo"
	"golang.org/x/sys/unix"
)

func Wrapper(ctx context.Context, handler string) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, unix.SIGINT)
	defer stop()

	if os.Getenv(antiRecursionEnv) != "" {
		panic("recursive execution detected!!!")
	}
	if !filepath.IsAbs(handler) {
		handler = filepath.Join(os.Getenv("LAMBDA_TASK_ROOT"), handler)
	}
	slog.InfoContext(ctx, "running child command", "command", handler)

	c := NewTelemetryAPIClient(fmt.Sprintf("http://127.0.0.1:%d", listenPort))
	cmd := exec.CommandContext(ctx, handler)
	cmd.Stdout = c
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Cancel = func() error {
		slog.InfoContext(ctx, "sending SIGTERM to child command")
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 1500 * time.Millisecond
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, antiRecursionEnv+"=1")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx = slogcontext.WithValue(ctx, "component", "telemetry-client")
		c.Run(ctx)
	}()

	err := cmd.Run()
	if err != nil {
		exitCode := wrapcommander.ResolveExitCode(err)
		slog.ErrorContext(ctx, "child command failed", "error", err, "exit_code", exitCode)
	}
	wg.Wait()
	return err
}

// TelemetryAPIClient is a client for sending telemetry data to the Firetap service.
// It implements the io.Writer interface.
type TelemetryAPIClient struct {
	w        io.Writer
	r        *bufio.Reader
	events   []TelemetryPostEvent
	endpoint string
	client   *http.Client
	mu       *sync.Mutex
}

func NewTelemetryAPIClient(endpoint string) *TelemetryAPIClient {
	client := &http.Client{
		Timeout: 1000 * time.Millisecond, // TODO: use TimeoutMS
	}
	r, w := io.Pipe()
	return &TelemetryAPIClient{
		w:        w,
		r:        bufio.NewReader(r),
		events:   make([]TelemetryPostEvent, 0, 500),
		endpoint: endpoint,
		client:   client,
		mu:       new(sync.Mutex),
	}
}

func (c *TelemetryAPIClient) Write(p []byte) (n int, err error) {
	slog.DebugContext(context.Background(), "writing", "bytes", len(p))
	return c.w.Write(p)
}

func (c *TelemetryAPIClient) readEvents(ctx context.Context) {
	ctx = slogcontext.WithValue(ctx, "component", "event-reader")
	for {
		line, err := c.r.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			slog.ErrorContext(ctx, "failed to read line", "error", err)
			break
		}
		c.mu.Lock()
		c.events = append(c.events, TelemetryPostEvent{
			Time:   time.Now().Format(time.RFC3339),
			Type:   "function",
			Record: line,
		})
		c.mu.Unlock()
	}
}

func (c *TelemetryAPIClient) Run(ctx context.Context) {
	defer slog.InfoContext(ctx, "telemetry client stopped")
	go c.readEvents(ctx)

	ctx = slogcontext.WithValue(ctx, "component", "telemetry-client")
	ticker := time.NewTicker(1000 * time.Millisecond) // TODO: use IntervalMS
	defer ticker.Stop()
	for {
		final := false
		select {
		case <-ticker.C:
		case <-ctx.Done():
			slog.InfoContext(ctx, "shutting down telemetry client")
			ctx = slogcontext.WithValue(context.Background(), "component", "telemetry-client")
			final = true
		}
		if sent, err := c.Post(ctx); err != nil {
			slog.ErrorContext(ctx, "failed to send telemetry", "error", err)
		} else {
			slog.DebugContext(ctx, "telemetry sent", "events", sent)
		}
		if final {
			break
		}
	}
}

func (c *TelemetryAPIClient) Post(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	slog.DebugContext(ctx, "telemetry", "events", len(c.events))
	if len(c.events) == 0 {
		return 0, nil
	}
	sent := 0
	for _, events := range lo.Chunk(c.events, 500) {
		slog.DebugContext(ctx, "sending telemetry", "events", len(events))
		buf := bufPool.Get().(*bytes.Buffer)
		defer bufPool.Put(buf)
		if err := json.NewEncoder(buf).Encode(events); err != nil {
			return sent, err
		}
		size := buf.Len()
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, buf)
		resp, err := c.client.Do(req)
		if err != nil {
			return sent, err
		}
		if resp.StatusCode != http.StatusOK {
			return sent, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		slog.InfoContext(ctx, "telemetry sent", "bytes", size)
		sent += len(events)
	}
	c.events = c.events[:0]
	return sent, nil
}

type TelemetryPostEvent struct {
	Time   string `json:"time"`
	Type   string `json:"type"`
	Record string `json:"record"`
}
