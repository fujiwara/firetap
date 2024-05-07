package firetap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type ExtensionClient struct {
	extensionId string
	client      *http.Client
	skip        bool
}

func NewExtensionClient() *ExtensionClient {
	s, _ := strconv.ParseBool(os.Getenv("FIRETAP_SKIP_EXTENSION"))
	logger.Info("extension client created", "skip", s)
	return &ExtensionClient{
		client: http.DefaultClient,
		skip:   s,
	}
}

func (c *ExtensionClient) Register(ctx context.Context) error {
	if c.skip {
		logger.Info("skipping extension registration")
		return nil
	}
	registerURL := fmt.Sprintf("%s/register", lambdaAPIEndpoint)
	req, _ := http.NewRequestWithContext(ctx, "POST", registerURL, strings.NewReader(`{"events":["INVOKE","SHUTDOWN"]}`))
	req.Header.Set(lambdaExtensionNameHeader, lambdaExtensionName)
	logger.Info("registering extension", "url", registerURL, "name", lambdaExtensionName, "headers", req.Header)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register extension: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode register response: %v", err)
	}
	logger.Info("register status", "status", resp.Status, "response", result)

	c.extensionId = resp.Header.Get(lambdaExtensionIdentifierHeader)
	if c.extensionId == "" {
		return fmt.Errorf("extension identifier is empty: %d %v", resp.StatusCode, resp.Header)
	}
	logger.Info("extension registered", "extension_id", c.extensionId)
	return nil
}

func (c *ExtensionClient) fetchNextEvent(ctx context.Context) (string, error) {
	u := fmt.Sprintf("%s/event/next", lambdaAPIEndpoint)
	logger.Debug("getting next event", "url", u, "extension_id", c.extensionId)
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
	logger.Debug("event received", "event", event)
	if ev, ok := event["eventType"].(string); ok {
		return ev, nil
	}
	return "", fmt.Errorf("eventType not found: %v", event)
}

func (c *ExtensionClient) Run(ctx context.Context, cancel func()) error {
	if c.skip {
		logger.Info("skipping extension running")
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
			logger.Debug("invoke event received")
		case "SHUTDOWN":
			logger.Debug("shutdown event received. shutting down extension")
			cancel()
			return nil
		}
	}
}
