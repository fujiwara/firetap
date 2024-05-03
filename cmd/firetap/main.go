package main

import (
	"context"
	"os"

	app "github.com/fujiwara/firetap"
)

func main() {
	ctx := context.TODO()
	if err := run(ctx); err != nil {
		app.Fatal(err)
	}
}

func run(ctx context.Context) error {
	if h := os.Getenv("_HANDLER"); h != "" {
		// in runtime
		app.LogType("firetap.runtime")
		return app.Wrapper(ctx, h)
	}
	// otherwise, in extension
	opt, err := app.NewOption()
	if err != nil {
		return err
	}
	app.LogType("firetap.extension")
	return app.Run(ctx, opt)
}
