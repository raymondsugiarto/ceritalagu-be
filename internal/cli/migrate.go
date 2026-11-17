package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/database"
)

// newDBCmd groups database commands: `server db migrate up|down|version|force`.
func newDBCmd() *cobra.Command {
	dbCmd := &cobra.Command{Use: "db", Short: "Database commands"}
	dbCmd.AddCommand(newMigrateCmd())
	return dbCmd
}

func newMigrateCmd() *cobra.Command {
	var schema string
	migrateCmd := &cobra.Command{Use: "migrate", Short: "Apply or inspect SQL migrations (db/migrations/postgres)"}
	migrateCmd.PersistentFlags().StringVar(&schema, "schema", "", "target schema (default database.main.schema; created if missing)")

	run := func(cmd *cobra.Command, fn func(*database.Migrator) error) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		return database.WithMigrator(cmd.Context(), cfg.Database.Main, schema, fn)
	}

	migrateCmd.AddCommand(
		&cobra.Command{
			Use:   "up [step]",
			Short: "Apply pending migrations (all, or the next N)",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				steps, err := optionalInt(args)
				if err != nil {
					return err
				}
				return run(cmd, func(m *database.Migrator) error { return m.Up(steps) })
			},
		},
		&cobra.Command{
			Use:   "down [step]",
			Short: "Roll back N migrations (default 1)",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				steps, err := optionalInt(args)
				if err != nil {
					return err
				}
				return run(cmd, func(m *database.Migrator) error { return m.Down(steps) })
			},
		},
		&cobra.Command{
			Use:   "version",
			Short: "Print the current migration version",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return run(cmd, func(m *database.Migrator) error {
					v, dirty, err := m.Version()
					if err != nil {
						return err
					}
					cmd.Printf("version %d (dirty=%t)\n", v, dirty)
					return nil
				})
			},
		},
		&cobra.Command{
			Use:   "force <version>",
			Short: "Set the version without running migrations (dirty-state recovery only)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				v, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("version must be a number: %w", err)
				}
				return run(cmd, func(m *database.Migrator) error { return m.Force(v) })
			},
		},
	)
	return migrateCmd
}

func optionalInt(args []string) (int, error) {
	if len(args) == 0 || args[0] == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("step must be a non-negative number, got %q", args[0])
	}
	return n, nil
}
