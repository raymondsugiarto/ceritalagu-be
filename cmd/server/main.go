// Command server is the binary entrypoint. It only runs the Cobra command tree in internal/cli;
// subcommands (serve, migrate, seed, ...) are added there.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/yourorg/music-api/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}
