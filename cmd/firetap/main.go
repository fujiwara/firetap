package main

import (
	"context"
	"log/slog"
	"os"

	app "github.com/fujiwara/firetap"
)

func main() {
	ctx := context.TODO()
	if err := run(ctx); err != nil {
		slog.Error("failed to run", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	if h := os.Getenv("_HANDLER"); h != "" {
		// in runtime
		app.SetLogger(slog.With("firetap", "runtime"))
		return app.Wrapper(ctx, h)
	}
	// otherwise, in extension
	app.SetLogger(slog.With("firetap", "extension"))
	return app.Run(ctx)
}
