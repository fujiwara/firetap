package firetap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fujiwara/firetap"
)

type testLogSender struct {
	logs []byte
}

func (s *testLogSender) Send(ctx context.Context, log []byte) error {
	s.logs = append(s.logs, log...)
	return nil
}

func (s *testLogSender) Flush(ctx context.Context) error {
	return nil
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

	if string(sender.logs) != strings.Repeat("foo\nbar\nbaz\n", 5) {
		t.Errorf("unexpected logs: %s", sender.logs)
	}
}
