package firetap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	slogcontext "github.com/PumpkinSeed/slog-context"
)

type Receiver struct {
	Endpoint string
}

func NewReceiver(ctx context.Context) (*Receiver, error) {
	receiver := &Receiver{
		Endpoint: fmt.Sprintf("http://sandbox.localdomain:%d", listenPort),
	}
	return receiver, nil
}

func (r *Receiver) Run(ctx context.Context, sender *LogSender) error {
	ctx = slogcontext.WithValue(ctx, "component", "receiver")

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", listenPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	slog.InfoContext(ctx, "receiver is listening", "addr", listener.Addr())

	m := http.NewServeMux()
	m.HandleFunc("/", handleTelemetry(sender))
	srv := http.Server{Handler: m}

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		log.Println("shutting down receiver")
		srv.Shutdown(ctx)
	}()
	if err := srv.Serve(listener); err != nil {
		if err != http.ErrServerClosed {
			return fmt.Errorf("failed to serve: %w", err)
		}
	}
	wg.Wait()
	return nil
}

func handleTelemetry(sender Sender) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := slogcontext.WithValue(r.Context(), "component", "handler")
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		dec := json.NewDecoder(r.Body)
		events := []TelemetryEvent{}
		if err := dec.Decode(&events); err != nil {
			slog.ErrorContext(ctx, "failed to decode request body", "error", err)
			http.Error(w, "failed to decode request body", http.StatusBadRequest)
			return
		}
		slog.InfoContext(ctx, "telemetry received", "events", len(events))
		var sent, ignored int
		for _, event := range events {
			slog.DebugContext(ctx, "telemetry received", "time", event.Time, "type", event.Type)
			if event.Record == nil {
				slog.WarnContext(ctx, "event record is empty")
				ignored++
				continue
			}
			record := event.Record
			switch event.Type {
			case "function":
				if b, err := restoreRecode(&record); err != nil {
					slog.WarnContext(ctx, "failed to restore record", "error", err, "record", string(record))
				} else {
					if err := sender.Send(ctx, b); err != nil {
						slog.WarnContext(ctx, "failed to send record", "error", err)
					} else {
						sent++
					}
				}
			default:
				ignored++
				// ignore unknown telemetry type
				// logger.Warn("unknown telemetry type", "type", event.Type, "record", string(record))
			}
		}
		slog.InfoContext(ctx, "logs sent", "sent", sent, "ignored", ignored)
		if err := sender.Flush(ctx); err != nil {
			slog.ErrorContext(ctx, "failed to flush", "error", err)
			http.Error(w, "failed to flush", http.StatusInternalServerError)
		}
	}
}

// TelemetryEvent represents an inbound Telemetry API message
// https://docs.aws.amazon.com/lambda/latest/dg/telemetry-api.html#telemetry-api-messages
type TelemetryEvent struct {
	Time   string          `json:"time"`
	Type   string          `json:"type"`
	Record json.RawMessage `json:"record"`
}

var bufPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func restoreRecode(b *json.RawMessage) ([]byte, error) {
	var s string
	if err := json.Unmarshal(*b, &s); err == nil {
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		return []byte(s), nil
	}
	buf := bufPool.Get().(*bytes.Buffer)
	defer bufPool.Put(buf)
	if err := json.Compact(buf, *b); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
