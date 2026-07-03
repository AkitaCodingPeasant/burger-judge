package app

import (
	"context"
	"os/signal"
	"syscall"

	"regs/internal/config"
)

// RunWorker is the composition root for the persistent job queue and judge
// runners. Repository and Docker implementations are wired here, not in the
// domain or use-case packages.
func RunWorker(_ config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}

