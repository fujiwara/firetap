package firetap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

type Receiver struct {
	Endpoint string
	listener net.Listener
}

func startReceiver() (*Receiver, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", listenPort))
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}
	logger.Info("receiver is listening", "addr", listener.Addr())

	receiver := &Receiver{
		listener: listener,
		Endpoint: fmt.Sprintf("http://sandbox.localdomain:%d", listenPort),
	}
	return receiver, nil
}

func (r *Receiver) Run(ctx context.Context, sender *LogSender) error {
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
	if err := srv.Serve(r.listener); err != nil {
		if err != http.ErrServerClosed {
			return fmt.Errorf("failed to serve: %w", err)
		}
	}
	wg.Wait()
	return nil
}

func handleTelemetry(sender *LogSender) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		dec := json.NewDecoder(r.Body)
		events := []TelemetryEvent{}
		if err := dec.Decode(&events); err != nil {
			logger.Error("failed to decode request body", "error", err)
			http.Error(w, "failed to decode request body", http.StatusBadRequest)
			return
		}
		logger.Info("telemetry received", "events", len(events))
		var sent, ignored int
		for _, event := range events {
			logger.Debug("telemetry received", "time", event.Time, "type", event.Type)
			if event.Record == nil {
				logger.Warn("event record is empty")
				ignored++
				continue
			}
			record := *event.Record
			switch event.Type {
			case "function":
				if b, err := restoreRecode(&record); err != nil {
					logger.Warn("failed to restore record", "error", err, "record", string(record))
				} else {
					if err := sender.Send(ctx, b); err != nil {
						logger.Warn("failed to send record", "error", err)
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
		logger.Info("logs sent", "sent", sent, "ignored", ignored)
		if err := sender.Flush(ctx); err != nil {
			logger.Error("failed to flush", "error", err)
			http.Error(w, "failed to flush", http.StatusInternalServerError)
		}
	}
}

type TelemetryEvent struct {
	Time   string           `json:"time"`
	Type   string           `json:"type"`
	Record *json.RawMessage `json:"record"`
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
