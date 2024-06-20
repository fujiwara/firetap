package firetap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fujiwara/firetap"
)

type testLogSender struct {
	logs []byte
	mu   sync.Mutex
}

func (s *testLogSender) Send(ctx context.Context, log []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, log...)
	return nil
}

func (s *testLogSender) Flush(ctx context.Context) error {
	return nil
}

func (s *testLogSender) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.logs)
}

func (s *testLogSender) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.logs)
}

func TestTelemetryAPI(t *testing.T) {
	sender := &testLogSender{}
	m := http.NewServeMux()
	m.HandleFunc("/", firetap.HandleTelemetry(sender))
	s := httptest.NewServer(m)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := firetap.NewTelemetryAPIClient(s.URL)
	go c.Run(ctx)

	for i := range 5 {
		time.Sleep(300 * time.Millisecond)
		for _, log := range []string{"foo\n", "bar\n", "baz\n"} {
			if n, err := c.Write([]byte(log)); n != len(log) || err != nil {
				t.Errorf("%d Write() = (%d, %v), want (%d, nil)", i, n, err, len(log))
			}
		}
	}
	time.Sleep(1000 * time.Millisecond)

	if sender.String() != strings.Repeat("foo\nbar\nbaz\n", 5) {
		t.Errorf("unexpected logs: %s", sender.logs)
	}
}
