package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/app"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := app.NewCommand().ExecuteContext(ctx); err != nil {
		logger.Error("Configra stopped", zap.Error(err))
		return 1
	}
	return 0
}
