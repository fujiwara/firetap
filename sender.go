package firetap

import (
	"context"
	"time"

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

type LogSender struct {
	streamName string
	buf        [][]byte
	bufSize    int
	firehose   *firehose.Client
	kinesis    *kinesis.Client
}

func startSender(ctx context.Context, streamName string, dataStream bool) (*LogSender, error) {
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

func (s *LogSender) Run(ctx context.Context, rch <-chan []byte) error {
	tk := time.NewTicker(1 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			s.flush(context.Background()) // flush remaining logs
			return nil
		case <-tk.C:
			s.flush(ctx)
		case msg := <-rch:
			if len(s.buf) == maxBatchSize || s.bufSize+len(msg) > 1024*512 {
				s.flush(ctx)
			}
			s.bufSize += len(msg)
			s.buf = append(s.buf, msg)
		}
	}
}

func (s *LogSender) flush(ctx context.Context) {
	if len(s.buf) == 0 {
		return
	}
	if s.kinesis != nil {
		s.flushToKinesis(ctx)
	} else {
		s.flushToFirehose(ctx)
	}
}

func (s *LogSender) flushToFirehose(ctx context.Context) {
	recs := make([]firehoseTypes.Record, 0, len(s.buf))
	for _, r := range s.buf {
		recs = append(recs, firehoseTypes.Record{Data: r})
	}
	logger.Debug("sending to firehose", "records", len(recs))

	err := retryPolicy.Do(ctx, func() error {
		_, err := s.firehose.PutRecordBatch(ctx, &firehose.PutRecordBatchInput{
			DeliveryStreamName: &s.streamName,
			Records:            recs,
		})
		return err
	})
	if err != nil {
		logger.Warn("failed to send to firehose", "error", err)
		for _, m := range s.buf {
			logger.Warn("overflow", "message", string(m))
		}
	} else {
		logger.Info("sent to firehose", "records", len(recs))
	}
	s.resetBuffer()
}

func (s *LogSender) flushToKinesis(ctx context.Context) {
	recs := make([]kinesisTypes.PutRecordsRequestEntry, 0, len(s.buf))
	for _, r := range s.buf {
		recs = append(recs, kinesisTypes.PutRecordsRequestEntry{Data: r})
	}

	logger.Debug("sending to kinesis", "records", len(recs))

	err := retryPolicy.Do(ctx, func() error {
		_, err := s.kinesis.PutRecords(ctx, &kinesis.PutRecordsInput{
			Records:    recs,
			StreamName: &s.streamName,
		})
		return err
	})
	if err != nil {
		logger.Warn("failed to send to kinesis", "error", err)
		for _, m := range s.buf {
			logger.Warn("overflow", "message", string(m))
		}
	} else {
		logger.Info("sent to kinesis", "records", len(recs))
	}
	s.resetBuffer()
}

func (s *LogSender) resetBuffer() {
	s.buf = s.buf[:0]
	s.bufSize = 0
}
