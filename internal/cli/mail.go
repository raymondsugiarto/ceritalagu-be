package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/logger"
	"github.com/yourorg/music-api/internal/mail"
)

func newMailCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "mail", Short: "Test the outgoing email setup"}
	cmd.AddCommand(newMailTestCmd())
	return cmd
}

func newMailTestCmd() *cobra.Command {
	var to, subject, body, attach string
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Send a test email (with an optional attachment) through the configured mail driver",
		Example: "  server mail test --to you@example.com\n" +
			"  server mail test --to you@example.com --attach ./song.mp3",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return sendTestMail(cmd.Context(), cmd.OutOrStdout(), to, subject, body, attach)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "recipient address (required)")
	cmd.Flags().StringVar(&subject, "subject", "Test email dari Lagu Untukmu", "subject line")
	cmd.Flags().StringVar(&body, "body", "Halo! Ini email percobaan.\n\n— Lagu Untukmu\n", "plain-text body")
	cmd.Flags().StringVar(&attach, "attach", "", "path to a file to attach (e.g. an mp3)")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// sendTestMail sends one email with the configured driver, so the setup (API key, sender, size
// limits) can be checked without generating a song.
func sendTestMail(ctx context.Context, out io.Writer, to, subject, body, attach string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	sender, err := mail.New(cfg.Mail, logger.New(cfg.Logger.Level, cfg.Logger.Format))
	if err != nil {
		return err
	}
	msg := mail.Message{To: to, Subject: subject, Text: body}
	if attach != "" {
		data, err := os.ReadFile(attach) //nolint:gosec // the path comes from the operator's own flag
		if err != nil {
			return fmt.Errorf("read attachment %s: %w", attach, err)
		}
		msg.Files = append(msg.Files, mail.Attachment{Name: filepath.Base(attach), Content: data})
	}
	if err := sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("send test email: %w", err)
	}
	_, _ = fmt.Fprintf(out, "test email sent to %s (driver %s, %d attachment(s))\n", to, cfg.Mail.Driver, len(msg.Files))
	return nil
}
