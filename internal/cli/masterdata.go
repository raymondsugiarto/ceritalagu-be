package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/database"
	"github.com/yourorg/music-api/internal/module/audit"
	"github.com/yourorg/music-api/internal/module/masterdata"
)

func newMasterDataCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "masterdata", Short: "Manage questionnaire option lists (genre, occasion, recipient type)"}
	cmd.AddCommand(&cobra.Command{
		Use:   "import-defaults",
		Short: "Add the default options whose code does not exist yet (never overwrites or re-adds archived codes)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			gdb, err := database.Open(cmd.Context(), cfg.Database.Main, "")
			if err != nil {
				return err
			}
			defer func() { _ = database.Close(gdb) }()
			svc := masterdata.NewService(masterdata.NewGormRepository(gdb), audit.NewGormRecorder(gdb))
			n, err := svc.ImportDefaults(cmd.Context())
			if err != nil {
				return fmt.Errorf("import defaults (%d added before the error): %w", n, err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "master data: %d default options added\n", n)
			return nil
		},
	})
	return cmd
}
