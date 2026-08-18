// Command kubectl-why explains why a Kubernetes resource is not working.
//
// It installs as a kubectl plugin, so `kubectl why pod api` and
// `kubectl-why pod api` are the same command.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/xavimf87/kubewhy/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
