// Command kimai-cli tracks time in Kimai from the command line.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/ngruychev/kimai-cli/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cmd.ExecuteContext(ctx)
}
