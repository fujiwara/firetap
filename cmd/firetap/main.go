package main

import (
	"context"
	"log/slog"
	"os"

	slogcontext "github.com/PumpkinSeed/slog-context"
	app "github.com/fujiwara/firetap"
)

func main() {
	ctx := context.TODO()
	if err := run(ctx); err != nil {
		slog.ErrorContext(ctx, err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	if h := os.Getenv("_HANDLER"); h != "" {
		// in runtime
		ctx = slogcontext.WithValue(ctx, "type", "firetap.wrapper")
		return app.Wrapper(ctx, h)
	}
	// otherwise, in extension
	opt, err := app.NewOption()
	if err != nil {
		return err
	}
	ctx = slogcontext.WithValue(ctx, "type", "firetap.extension")
	return app.Run(ctx, opt)
}
