package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/hibiken/asynq"
	"github.com/spf13/cobra"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/app"
	"github.com/yourorg/music-api/internal/database"
	"github.com/yourorg/music-api/internal/logger"
	"github.com/yourorg/music-api/internal/mail"
	"github.com/yourorg/music-api/internal/module/admin"
	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/aiprovider/lyria"
	"github.com/yourorg/music-api/internal/module/aiprovider/minimax"
	"github.com/yourorg/music-api/internal/module/aiprovider/muapi"
	"github.com/yourorg/music-api/internal/module/aiprovider/sunoapi"
	"github.com/yourorg/music-api/internal/module/audit"
	"github.com/yourorg/music-api/internal/module/auth"
	"github.com/yourorg/music-api/internal/module/contact"
	"github.com/yourorg/music-api/internal/module/generation"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/masterdata"
	"github.com/yourorg/music-api/internal/module/payment"
	"github.com/yourorg/music-api/internal/module/payment/doku"
	"github.com/yourorg/music-api/internal/module/playground"
	"github.com/yourorg/music-api/internal/module/pricing"
	"github.com/yourorg/music-api/internal/module/prompt"
	"github.com/yourorg/music-api/internal/module/questionnaire"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/module/user"
	"github.com/yourorg/music-api/internal/secret"
	"github.com/yourorg/music-api/internal/storage"
)

// adminID gives modules the signed-in admin's ID for their audit log.
func adminID(c fiber.Ctx) string { return admin.FromCtx(c).ID }

// adminSessionTTL is how long an admin stays signed in.
const adminSessionTTL = 12 * time.Hour

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context())
		},
	}
}

// serve runs config → logger → modules → app → listen; on SIGINT/SIGTERM it shuts down gracefully.
func serve(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.Logger.Level, cfg.Logger.Format)
	slog.SetDefault(log) // modules without an injected logger (audit failures) log through it

	// ---- Database ----
	if cfg.Database.Main.AutoMigrate {
		if err := database.WithMigrator(ctx, cfg.Database.Main, "", func(m *database.Migrator) error { return m.Up(0) }); err != nil {
			return err
		}
		log.Info("database migrated", "schema", cfg.Database.Main.Schema)
	}
	gdb, err := database.Open(ctx, cfg.Database.Main, "")
	if err != nil {
		return err
	}
	defer func() { _ = database.Close(gdb) }()

	// ---- Queue (Asynq client; the worker is `server worker`) ----
	queueClient := asynq.NewClient(redisOpt(cfg.Redis))
	defer func() { _ = queueClient.Close() }()
	gen := cfg.Generation
	enqueuer := generation.NewEnqueuer(queueClient, gen.Queue, gen.TaskTimeout, gen.TaskMaxRetry)

	box, err := secret.NewBox(cfg.Security.MasterKey)
	if err != nil {
		return fmt.Errorf("security.masterKey (SECURITY_MASTERKEY): %w", err)
	}

	store, err := storage.New(cfg.Storage)
	if err != nil {
		return err
	}
	log.Info("audio storage", "driver", cfg.Storage.Driver, "bucket", cfg.Storage.S3.Bucket)

	mailer, err := mail.New(cfg.Mail, log)
	if err != nil {
		return err
	}
	log.Info("mail", "driver", cfg.Mail.Driver)

	// ---- Modules (manual DI: explicit and greppable) ----
	adminSvc := admin.NewService(admin.NewGormRepository(gdb), admin.NewSigner(cfg.Security.MasterKey, adminSessionTTL))
	guard := admin.NewGuard(adminSvc)
	adapters := aiprovider.NewRegistry()
	minimax.Register(adapters)
	lyria.Register(adapters)
	muapi.Register(adapters)
	sunoapi.Register(adapters)
	providerSvc := aiprovider.NewService(aiprovider.NewGormRepository(gdb), box, audit.NewGormRecorder(gdb),
		adapters, catalog())
	promptStore := prompt.NewGormRepository(gdb)
	promptSvc := prompt.NewService(promptStore,
		generation.NewLyricsTester(generation.NewProviderResolver(aiprovider.NewGormRepository(gdb), box), adapters),
		audit.NewGormRecorder(gdb))
	playgroundSvc := playground.NewService(playground.NewGormRepository(gdb), store)
	promptSvc.WithPlaygroundSaver(playgroundSvc)
	userRepo := user.NewGormRepository(gdb)
	userSvc := user.NewService(userRepo)
	authSvc := auth.NewService(auth.NewGormRepository(gdb), userRepo, guest.NewGormRepository(gdb), mailer,
		auth.NewKeys(cfg.Security.MasterKey))
	masterSvc := masterdata.NewService(masterdata.NewGormRepository(gdb), audit.NewGormRecorder(gdb))
	questionnaireSvc := questionnaire.NewService(questionnaire.NewGormRepository(gdb), guest.NewGormRepository(gdb), masterSvc, userSvc)
	songSvc := song.NewService(song.NewGormRepository(gdb), questionnaireSvc, enqueuer,
		store, song.Limits{GuestDailyQuota: gen.GuestDailyQuota, MaxRegenerate: gen.MaxRegenerate,
			SpareReveal: gen.SpareReveal})

	// Payments stay off (checkout answers 503) until the DOKU credentials are configured.
	var gateway payment.Gateway
	if d := cfg.Payment.Doku; d.Enabled() {
		gateway = doku.New(d.BaseURL, d.ClientID, d.SecretKey)
	}
	log.Info("payment", "gateway", "doku", "enabled", gateway != nil, "base_url", cfg.Payment.Doku.BaseURL)
	// Prices are edited by admins (setting song_pricing); payment.unlockPrice is the default until then.
	pricingSvc := pricing.NewService(pricing.NewGormRepository(gdb), audit.NewGormRecorder(gdb),
		pricing.Prices{Normal: cfg.Payment.UnlockPrice, Sale: cfg.Payment.UnlockPrice, MinPayable: pricing.DefaultMinPayable})
	paymentSvc := payment.NewService(payment.NewGormRepository(gdb), songSvc, userRepo, pricingSvc, gateway, payment.Settings{
		OrderTTL:      cfg.Payment.OrderTTL,
		PublicBaseURL: cfg.Payment.PublicBaseURL, NotifyPath: cfg.Payment.Doku.NotifyPath,
	}, log)

	srv := app.New(cfg, log, app.NewHealth(database.NewChecker(gdb), newRedisChecker(cfg.Redis)),
		guest.NewMiddleware(cfg.Security.IPHashSalt, cfg.Security.CookieSecure), // first: every route below needs the visitor
		questionnaire.NewHandler(questionnaireSvc),
		song.NewHandler(songSvc),
		payment.NewHandler(paymentSvc),
		contact.NewHandler(contact.NewService(mailer, cfg.Mail.ContactInbox(), log)),
		song.NewAdminHandler(song.NewAdminService(song.NewGormRepository(gdb)), guard.Require(admin.RoleSuperAdmin)),
		song.NewAdminSongHandler(song.NewAdminSongService(song.NewGormRepository(gdb), store), guard.Require(admin.RoleSuperAdmin)),
		auth.NewHandler(authSvc, cfg.Security.CookieSecure),
		user.NewHandler(userSvc, guard.Require(admin.RoleSuperAdmin)),
		admin.NewHandler(adminSvc, guard, cfg.Security.CookieSecure),
		aiprovider.NewHandler(providerSvc, guard.Require(admin.RoleSuperAdmin), adminID),
		masterdata.NewHandler(masterSvc, guard.Require(admin.RoleSuperAdmin), adminID),
		pricing.NewHandler(pricingSvc, guard.Require(admin.RoleSuperAdmin), adminID),
		prompt.NewHandler(promptSvc, guard.Require(admin.RoleSuperAdmin), adminID),
		playground.NewHandler(playgroundSvc, guard.Require(admin.RoleSuperAdmin)),
	)

	// ---- Lifecycle ----
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Listen() }()

	select {
	case err := <-errCh:
		return fmt.Errorf("server stopped: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.Rest.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Info("server stopped cleanly")
	return nil
}

// catalog lists every adapter an admin can pick in the provider form.
func catalog() []aiprovider.AdapterInfo {
	var out []aiprovider.AdapterInfo
	out = append(out, minimax.Catalog()...)
	out = append(out, lyria.Catalog()...)
	out = append(out, muapi.Catalog()...)
	return append(out, sunoapi.Catalog()...)
}
