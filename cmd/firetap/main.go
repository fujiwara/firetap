package main

import (
	"context"

	app "github.com/fujiwara/firetap"
)

func main() {
	ctx := context.TODO()
	if err := run(ctx); err != nil {
		app.Fatal(err)
	}
}

func run(ctx context.Context) error {
	opt, err := app.NewOption()
	if err != nil {
		return err
	}
	app.LogType("firetap.extension")
	return app.Run(ctx, opt)
}
