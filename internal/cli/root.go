// Package cli defines the command-line interface (Cobra). Each subcommand lives in its own file
// and only wires dependencies; business logic stays in internal/module and friends.
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// Set at build time: -ldflags "-X github.com/yourorg/music-api/internal/cli.version=v1.2.3".
var version = "dev"

// NewRootCmd builds the command tree. Add a subcommand with root.AddCommand(newXxxCmd()).
func NewRootCmd() *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:           "server",
		Short:         "Personalized song generator API",
		Version:       version,
		SilenceUsage:  true, // don't print usage on runtime errors, only on bad flags/args
		SilenceErrors: true, // main prints the error once
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			// --config is a shortcut for CONFIG_PATH; the config package stays the only reader.
			if configPath != "" {
				return os.Setenv("CONFIG_PATH", configPath)
			}
			return nil
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "directory holding *.yml config (overrides CONFIG_PATH)")

	root.AddCommand(newServeCmd(), newWorkerCmd(), newDBCmd(), newAdminCmd(), newMasterDataCmd(), newMailCmd())
	return root
}
