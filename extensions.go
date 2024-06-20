package firetap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	slogcontext "github.com/PumpkinSeed/slog-context"
)

type ExtensionClient struct {
	extensionId string
	client      *http.Client
	skip        bool
}

func NewExtensionClient(ctx context.Context) *ExtensionClient {
	var s bool
	if strings.HasPrefix(os.Getenv("AWS_EXECUTION_ENV"), "AWS_Lambda") || os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		slog.DebugContext(ctx, "running in AWS Lambda environment")
	} else {
		slog.InfoContext(ctx, "running outside of AWS Lambda environment, skipping extension client")
		s = true
	}
	return &ExtensionClient{
		client: http.DefaultClient,
		skip:   s,
	}
}

func (c *ExtensionClient) Register(ctx context.Context) error {
	ctx = slogcontext.WithValue(ctx, "component", "extension-client")
	if c.skip {
		slog.InfoContext(ctx, "skipping extension registration")
		return nil
	}
	registerURL := fmt.Sprintf("%s/register", lambdaExtensionAPIEndpoint)
	req, _ := http.NewRequestWithContext(ctx, "POST", registerURL, strings.NewReader(`{"events":["SHUTDOWN"]}`))
	req.Header.Set(lambdaExtensionNameHeader, lambdaExtensionName)
	slog.InfoContext(ctx, "registering extension", "url", registerURL, "name", lambdaExtensionName, "headers", req.Header)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register extension: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode register response: %v", err)
	}
	slog.InfoContext(ctx, "register status", "status", resp.Status, "response", result)

	c.extensionId = resp.Header.Get(lambdaExtensionIdentifierHeader)
	if c.extensionId == "" {
		return fmt.Errorf("extension identifier is empty: %d %v", resp.StatusCode, resp.Header)
	}
	slog.InfoContext(ctx, "extension registered", "extension_id", c.extensionId)
	return nil
}

func (c *ExtensionClient) fetchNextEvent(ctx context.Context) (string, error) {
	u := fmt.Sprintf("%s/event/next", lambdaExtensionAPIEndpoint)
	slog.DebugContext(ctx, "getting next event", "url", u, "extension_id", c.extensionId)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set(lambdaExtensionIdentifierHeader, c.extensionId)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get next event: %v", err)
	}
	defer resp.Body.Close()
	var event map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return "", fmt.Errorf("failed to decode event: %v", err)
	}
	slog.DebugContext(ctx, "event received", "event", event)
	if ev, ok := event["eventType"].(string); ok {
		return ev, nil
	}
	return "", fmt.Errorf("eventType not found: %v", event)
}

func (c *ExtensionClient) Run(ctx context.Context, cancel func()) error {
	ctx = slogcontext.WithValue(ctx, "component", "extension-client")
	if c.skip {
		slog.InfoContext(ctx, "skipping extension running")
		<-ctx.Done()
		return nil
	}
	for {
		ev, err := c.fetchNextEvent(ctx)
		if err != nil {
			return err
		}
		switch ev {
		case "INVOKE":
			slog.DebugContext(ctx, "invoke event received")
		case "SHUTDOWN":
			slog.DebugContext(ctx, "shutdown event received. shutting down extension")
			cancel()
			return nil
		}
	}
}

func (c *ExtensionClient) SubscribeTelemetry(ctx context.Context, endpoint string) error {
	ctx = slogcontext.WithValue(ctx, "component", "extension-client")
	if c.skip {
		slog.InfoContext(ctx, "skipping extension subscription to telemetry")
		return nil
	}
	u := lambdaTelemetryAPIEndpoint
	payload := NewTelemetrySubscription(endpoint)
	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "PUT", u, bytes.NewReader(jsonPayload))
	req.Header.Set(lambdaExtensionNameHeader, lambdaExtensionName)
	req.Header.Set(lambdaExtensionIdentifierHeader, c.extensionId)
	slog.InfoContext(ctx, "subscribing telemetry", "url", u, "name", lambdaExtensionName, "headers", req.Header, "payload", payload)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register extension: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to subscribe telemetry: %d %s", resp.StatusCode, string(b))
	} else {
		slog.InfoContext(ctx, "subscribed telemetry", "status", resp.Status, "response", string(b))
	}
	return nil
}

type TelemetrySubscription struct {
	SchemaVersion string               `json:"schemaVersion"`
	Types         []string             `json:"types"`
	Buffering     TelemetryBuffering   `json:"buffering"`
	Destination   TelemetryDestination `json:"destination"`
}

type TelemetryBuffering struct {
	MaxItems  int `json:"maxItems"`
	MaxBytes  int `json:"maxBytes"`
	TimeoutMs int `json:"timeoutMs"`
}

type TelemetryDestination struct {
	Protocol string `json:"protocol"`
	URI      string `json:"URI"`
}

func NewTelemetrySubscription(endpoint string) *TelemetrySubscription {
	return &TelemetrySubscription{
		SchemaVersion: "2022-12-13",
		Types:         []string{"function", "platform"},
		Buffering: TelemetryBuffering{
			MaxItems:  500,
			MaxBytes:  1024 * 1024,
			TimeoutMs: 1000,
		},
		Destination: TelemetryDestination{
			Protocol: "HTTP",
			URI:      endpoint,
		},
	}
}

/*
{
   "schemaVersion": "2022-12-13",
   "types": [
        "platform",
        "function",
        "extension"
   ],
   "buffering": {
        "maxItems": 1000,
        "maxBytes": 256*1024,
        "timeoutMs": 100
   },
   "destination": {
        "protocol": "HTTP",
        "URI": "http://sandbox.localdomain:8080"
   }
}*/
