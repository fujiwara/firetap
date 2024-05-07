package firetap

import (
	"fmt"
	"os"
	"strconv"
)

type Option struct {
	StreamName string // StreamName is the name of the Kinesis Firehose/DataStream stream name.
	DataStream bool   // DataStream is the flag to use DataStream instead of Firehose.
}

func NewOption() (*Option, error) {
	ds, _ := strconv.ParseBool(os.Getenv("FIRETAP_DATA_STREAM"))
	opt := &Option{
		StreamName: os.Getenv("FIRETAP_STREAM_NAME"),
		DataStream: ds,
	}
	if opt.StreamName == "" {
		return nil, fmt.Errorf("FIRETAP_STREAM_NAME is required")
	}
	return opt, nil
}
