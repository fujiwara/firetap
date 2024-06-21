package firetap

import (
	"log/slog"
	"os"

	slogcontext "github.com/PumpkinSeed/slog-context"
)

var LogLevel = new(slog.LevelVar)

func init() {
	opts := &slog.HandlerOptions{Level: LogLevel}
	slog.SetDefault(slog.New(slogcontext.NewHandler(slog.NewJSONHandler(os.Stderr, opts))))
}
