package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/app"
)

// Set by the release build. Development builds keep explicit non-release values.
var version = "dev"
var commit = "unknown"

func main() {
	os.Exit(run())
}

func run() int {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	command := app.NewCommand()
	command.Version = fmt.Sprintf("%s (%s)", version, commit)
	if err := command.ExecuteContext(ctx); err != nil {
		logger.Error("Configra stopped", zap.Error(err))
		return 1
	}
	return 0
}
