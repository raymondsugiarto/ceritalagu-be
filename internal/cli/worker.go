package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/spf13/cobra"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/audio"
	"github.com/yourorg/music-api/internal/database"
	"github.com/yourorg/music-api/internal/logger"
	"github.com/yourorg/music-api/internal/mail"
	"github.com/yourorg/music-api/internal/module/aiprovider"
	"github.com/yourorg/music-api/internal/module/aiprovider/lyria"
	"github.com/yourorg/music-api/internal/module/aiprovider/minimax"
	"github.com/yourorg/music-api/internal/module/aiprovider/muapi"
	"github.com/yourorg/music-api/internal/module/aiprovider/sunoapi"
	"github.com/yourorg/music-api/internal/module/generation"
	"github.com/yourorg/music-api/internal/module/notification"
	"github.com/yourorg/music-api/internal/module/prompt"
	"github.com/yourorg/music-api/internal/module/questionnaire"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/module/user"
	"github.com/yourorg/music-api/internal/secret"
	"github.com/yourorg/music-api/internal/storage"
)

// Compile-time checks that the GORM repositories satisfy the pipeline's interfaces.
var (
	_ generation.Jobs           = (*song.GormRepository)(nil)
	_ generation.Questionnaires = (*questionnaire.GormRepository)(nil)
	_ generation.Prompts        = (*prompt.GormRepository)(nil)
	_ generation.ProviderStore  = (*aiprovider.GormRepository)(nil)
)

func newWorkerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Process song generation jobs from the Asynq queue",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return work(cmd.Context())
		},
	}
}

// work runs config → logger → DB → pipeline → Asynq server; on SIGINT/SIGTERM it stops taking
// new tasks and waits for running ones.
func work(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.Logger.Level, cfg.Logger.Format)
	box, err := secret.NewBox(cfg.Security.MasterKey)
	if err != nil {
		return fmt.Errorf("security.masterKey (SECURITY_MASTERKEY): %w", err)
	}

	gdb, err := database.Open(ctx, cfg.Database.Main, "")
	if err != nil {
		return err
	}
	defer func() { _ = database.Close(gdb) }()

	registry := aiprovider.NewRegistry()
	minimax.Register(registry)
	lyria.Register(registry)
	muapi.Register(registry)
	sunoapi.Register(registry)

	store, err := storage.New(cfg.Storage)
	if err != nil {
		return err
	}

	gen := cfg.Generation

	// The song-ready email is optional: without a mailer (or with notifications off) the
	// pipeline simply skips it.
	var notifier generation.Notifier
	if cfg.Notification.Enabled {
		mailer, err := mail.New(cfg.Mail, log)
		if err != nil {
			return err
		}
		notifier = notification.NewService(mailer, user.NewGormRepository(gdb), store,
			notification.Setting{
				Enabled:            cfg.Notification.Enabled,
				AttachFullAudio:    cfg.Notification.AttachFullAudio,
				MaxAttachmentBytes: cfg.Notification.MaxAttachmentMB * 1024 * 1024,
				SiteURL:            cfg.Notification.SiteURL,
			}, log)
		log.Info("song-ready email", "enabled", true, "driver", cfg.Mail.Driver,
			"attach_full_audio", cfg.Notification.AttachFullAudio)
	}

	pipeline := generation.NewPipeline(generation.Deps{
		Jobs:           song.NewGormRepository(gdb),
		Questionnaires: questionnaire.NewGormRepository(gdb),
		Prompts:        prompt.NewGormRepository(gdb),
		Providers:      generation.NewProviderResolver(aiprovider.NewGormRepository(gdb), box),
		Registry:       registry,
		Storage:        store,
		Audio:          audio.NewFFmpeg(gen.FFmpegPath),
		Notifier:       notifier,
		Log:            log,
	}, generation.Options{
		PreviewSeconds: gen.PreviewSeconds, FadeSeconds: gen.FadeSeconds,
		GuestRetention: gen.GuestRetention, RetryBaseDelay: gen.RetryBaseDelay,
	})

	srv := asynq.NewServer(redisOpt(cfg.Redis), asynq.Config{
		Concurrency: gen.Concurrency,
		Queues:      map[string]int{gen.Queue: 1},
		Logger:      asynqLogger{log: log},
		BaseContext: func() context.Context { return ctx },
	})
	mux := asynq.NewServeMux()
	mux.Handle(generation.TaskGenerateSong, generation.NewTaskHandler(pipeline))

	if err := srv.Start(mux); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}
	log.Info("worker started", "queue", gen.Queue, "concurrency", gen.Concurrency)

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()
	log.Info("shutdown signal received, waiting for running jobs")
	srv.Shutdown()
	log.Info("worker stopped cleanly")
	return nil
}

func redisOpt(c config.RedisConfig) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: c.Addr, Password: c.Password, DB: c.DB}
}

// asynqLogger routes Asynq's internal logs through slog.
type asynqLogger struct {
	log *slog.Logger
}

func (l asynqLogger) Debug(args ...any) { l.log.Debug(fmt.Sprint(args...), "component", "asynq") }
func (l asynqLogger) Info(args ...any)  { l.log.Info(fmt.Sprint(args...), "component", "asynq") }
func (l asynqLogger) Warn(args ...any)  { l.log.Warn(fmt.Sprint(args...), "component", "asynq") }
func (l asynqLogger) Error(args ...any) { l.log.Error(fmt.Sprint(args...), "component", "asynq") }
func (l asynqLogger) Fatal(args ...any) {
	l.log.Error(fmt.Sprint(args...), "component", "asynq", "fatal", true)
	os.Exit(1) //nolint:revive // asynq.Logger contract: Fatal must not return
}
