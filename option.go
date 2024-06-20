package firetap

import (
	"log/slog"

	"github.com/alecthomas/kong"
)

type Option struct {
	StreamName string `help:"Firehose or DataStream name" env:"FIRETAP_STREAM_NAME" required:""`
	DataStream bool   `help:"The flag to use DataStream instead of Firehose" env:"FIRETAP_DATA_STREAM" default:"false"`
	Port       int    `help:"The port to listen on" default:"8080" env:"FIRETAP_PORT"`
	Debug      bool   `help:"Enable debug mode" env:"FIRETAP_DEBUG" default:"false"`
}

func NewOption() (*Option, error) {
	opt := &Option{}
	kong.Parse(opt)
	if opt.Debug {
		LogLevel.Set(slog.LevelDebug)
	}
	return opt, nil
}
