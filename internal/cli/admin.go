package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/database"
	"github.com/yourorg/music-api/internal/module/admin"
)

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Manage back-office admin accounts"}
	cmd.AddCommand(newAdminCreateCmd())
	return cmd
}

func newAdminCreateCmd() *cobra.Command {
	var in admin.CreateAdminInput
	var role string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an admin; the password is read from the terminal (hidden) or from stdin",
		Example: "  server admin create --email ops@example.com --name \"Ops\"\n" +
			"  printf '%s\\n' \"$ADMIN_PASSWORD\" | server admin create --email ops@example.com --name Ops",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pw, err := readPassword(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			in.Password, in.Role = pw, admin.Role(role)
			return createAdmin(cmd.Context(), cmd.OutOrStdout(), in)
		},
	}
	cmd.Flags().StringVar(&in.Email, "email", "", "login email (required)")
	cmd.Flags().StringVar(&in.Name, "name", "", "display name (required)")
	cmd.Flags().StringVar(&role, "role", string(admin.RoleSuperAdmin), "super_admin | operator")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func createAdmin(ctx context.Context, out io.Writer, in admin.CreateAdminInput) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	gdb, err := database.Open(ctx, cfg.Database.Main, "")
	if err != nil {
		return err
	}
	defer func() { _ = database.Close(gdb) }()

	// The signer is unused here; Create only needs the repository.
	svc := admin.NewService(admin.NewGormRepository(gdb), admin.NewSigner(cfg.Security.MasterKey, 0))
	a, err := svc.Create(ctx, in)
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	_, _ = fmt.Fprintf(out, "admin created: %s <%s> role=%s id=%s\n", a.Name, a.Email, a.Role, a.ID)
	return nil
}

// readPassword asks twice on a terminal (no echo); otherwise it reads one line from stdin.
func readPassword(prompt io.Writer) (string, error) {
	fd := int(os.Stdin.Fd()) //nolint:gosec // a file descriptor always fits in int
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	ask := func(label string) (string, error) {
		_, _ = fmt.Fprint(prompt, label)
		b, err := term.ReadPassword(fd)
		_, _ = fmt.Fprintln(prompt)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return string(b), nil
	}
	pw, err := ask(fmt.Sprintf("Password (min %d characters): ", admin.MinPasswordLength))
	if err != nil {
		return "", err
	}
	again, err := ask("Repeat password: ")
	if err != nil {
		return "", err
	}
	if pw != again {
		return "", errors.New("passwords do not match")
	}
	return pw, nil
}
