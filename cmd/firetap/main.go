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
	return app.Run(ctx)
}
