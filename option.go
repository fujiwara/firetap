package firetap

import (
	"fmt"
	"os"
)

type Option struct {
	StreamName string // StreamName is the name of the Kinesis Firehose stream.
}

func NewOption() (*Option, error) {
	opt := &Option{
		StreamName: os.Getenv("FIRETAP_STREAM_NAME"),
	}
	if opt.StreamName == "" {
		return nil, fmt.Errorf("FIRETAP_STREAM_NAME is required")
	}
	return opt, nil
}
