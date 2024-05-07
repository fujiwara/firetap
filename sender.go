package firetap

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	"github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"github.com/shogo82148/go-retry"
)

const maxFirehoseBatchSize = 500

var firehoseRetryPolicy = retry.Policy{
	MinDelay: 100 * time.Millisecond,
	MaxDelay: 2 * time.Second,
	MaxCount: 10,
}

type LogSender struct {
	stream   string
	buf      [][]byte
	firehose *firehose.Client
}

func startSender(ctx context.Context, stream string) (*LogSender, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	firehoseClient := firehose.NewFromConfig(awsCfg)

	return &LogSender{
		stream:   stream,
		firehose: firehoseClient,
		buf:      make([][]byte, 0, maxFirehoseBatchSize),
	}, nil
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
			if len(s.buf) == maxFirehoseBatchSize {
				s.flush(ctx)
			}
			s.buf = append(s.buf, msg)
		}
	}
}

func (s *LogSender) flush(ctx context.Context) {
	if len(s.buf) == 0 {
		return
	}
	recs := make([]types.Record, 0, len(s.buf))
	for _, r := range s.buf {
		recs = append(recs, types.Record{Data: r})
	}
	logger.Debug("sending to firehose", "records", len(recs))

	err := firehoseRetryPolicy.Do(ctx, func() error {
		_, err := s.firehose.PutRecordBatch(ctx, &firehose.PutRecordBatchInput{
			DeliveryStreamName: &s.stream,
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
	s.buf = s.buf[:0] // clear buffer
}
