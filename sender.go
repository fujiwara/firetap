package firetap

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	slogcontext "github.com/PumpkinSeed/slog-context"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	firehoseTypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesisTypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"github.com/shogo82148/go-retry"
)

const maxBatchSize = 500

var retryPolicy = retry.Policy{
	MinDelay: 100 * time.Millisecond,
	MaxDelay: 2 * time.Second,
	MaxCount: 10,
}

type Sender interface {
	Send(ctx context.Context, msg []byte) error
	Flush(ctx context.Context) error
}

type LogSender struct {
	streamName string
	buf        [][]byte
	bufSize    int
	firehose   *firehose.Client
	kinesis    *kinesis.Client
	mu         sync.Mutex
}

func NewSender(ctx context.Context, streamName string, dataStream bool) (*LogSender, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	s := &LogSender{
		streamName: streamName,
		buf:        make([][]byte, 0, maxBatchSize),
	}
	if dataStream {
		s.kinesis = kinesis.NewFromConfig(awsCfg)
	} else {
		s.firehose = firehose.NewFromConfig(awsCfg)
	}
	return s, nil
}

func (s *LogSender) Send(ctx context.Context, msg []byte) error {
	ctx = slogcontext.WithValue(ctx, "component", "sender")
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) == maxBatchSize || s.bufSize+len(msg) > 1024*512 {
		if err := s.Flush(ctx); err != nil {
			return fmt.Errorf("failed to flush: %w", err)
		}
	}
	s.bufSize += len(msg)
	s.buf = append(s.buf, msg)
	return nil
}

func (s *LogSender) Flush(ctx context.Context) error {
	ctx = slogcontext.WithValue(ctx, "stream", s.streamName)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) == 0 {
		return nil
	}
	if s.kinesis != nil {
		return s.flushToKinesis(ctx)
	} else {
		return s.flushToFirehose(ctx)
	}
}

func (s *LogSender) flushToFirehose(ctx context.Context) error {
	recs := make([]firehoseTypes.Record, 0, len(s.buf))
	for _, r := range s.buf {
		recs = append(recs, firehoseTypes.Record{Data: r})
	}
	slog.DebugContext(ctx, "sending to firehose", "records", len(recs))

	err := retryPolicy.Do(ctx, func() error {
		_, err := s.firehose.PutRecordBatch(ctx, &firehose.PutRecordBatchInput{
			DeliveryStreamName: &s.streamName,
			Records:            recs,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to send to firehose: %w", err)
	} else {
		slog.InfoContext(ctx, "sent to firehose", "records", len(recs))
	}
	s.resetBuffer()
	return nil
}

func (s *LogSender) flushToKinesis(ctx context.Context) error {
	recs := make([]kinesisTypes.PutRecordsRequestEntry, 0, len(s.buf))
	for _, r := range s.buf {
		recs = append(recs, kinesisTypes.PutRecordsRequestEntry{Data: r})
	}
	slog.DebugContext(ctx, "sending to kinesis", "records", len(recs))

	err := retryPolicy.Do(ctx, func() error {
		_, err := s.kinesis.PutRecords(ctx, &kinesis.PutRecordsInput{
			Records:    recs,
			StreamName: &s.streamName,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to send to kinesis: %w", err)
	} else {
		slog.InfoContext(ctx, "sent to kinesis", "records", len(recs))
	}
	s.resetBuffer()
	return nil
}

func (s *LogSender) resetBuffer() {
	s.buf = s.buf[:0]
	s.bufSize = 0
}
