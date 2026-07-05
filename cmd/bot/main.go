package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bot/internal/config"
	"bot/internal/domain"
	"bot/internal/feature/group"
	"bot/internal/feature/owner"
	"bot/internal/feature/user"
	"bot/internal/logging"
	"bot/internal/store"
	"bot/internal/telegram"
)

const (
	mongoConnectTimeout     = 10 * time.Second
	mongoIndexTimeout       = 5 * time.Second
	mongoDisconnectTimeout  = 5 * time.Second
	ownerBootstrapTimeout   = 5 * time.Second
	telegramShutdownTimeout = 10 * time.Second
)

var processStart = time.Now()

func main() {
	configOnly := flag.Bool("config-only", false, "load and print configuration then exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		logging.Error("configuration error", logging.Fields{"error": err})
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.Setup(cfg)
	if err != nil {
		logging.Error("logger setup error", logging.Fields{"error": err})
		fmt.Fprintf(os.Stderr, "logger setup error: %v\n", err)
		os.Exit(1)
	}

	if *configOnly {
		logging.Info("configuration check", logging.Fields{"event": "config_only"})
		fmt.Println("configuration check: ok")
		fmt.Println(config.FormatRedacted(cfg))
		return
	}

	logger.WithFields(logging.Fields{
		"event":    "startup",
		"mongo_db": cfg.MongoDB,
	}).Info("configuration loaded")

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), mongoConnectTimeout)
	mongoManager, err := store.NewManager(connectCtx, cfg)
	cancelConnect()
	if err != nil {
		logger.WithError(err).Error("mongo connection error")
		fmt.Fprintf(os.Stderr, "mongo connection error: %v\n", err)
		os.Exit(1)
	}

	logger.WithField("event", "mongo_connect").Info("connected to mongo")

	indexCtx, cancelIndexes := context.WithTimeout(context.Background(), mongoIndexTimeout)
	if err := mongoManager.EnsureBaseIndexes(indexCtx); err != nil {
		cancelIndexes()
		logger.WithError(err).Error("mongo index setup error")
		fmt.Fprintf(os.Stderr, "mongo index setup error: %v\n", err)
		os.Exit(1)
	}
	cancelIndexes()

	logger.WithField("event", "mongo_indexes").Info("ensured base mongo indexes")

	ownerRegistrar := owner.NewRegistrar(mongoManager.Users(), logger)
	ownerCtx, cancelOwner := context.WithTimeout(context.Background(), ownerBootstrapTimeout)
	if err := ownerRegistrar.EnsureOwner(ownerCtx, cfg.BotOwnerID); err != nil {
		cancelOwner()
		logger.WithError(err).Error("owner bootstrap error")
		fmt.Fprintf(os.Stderr, "owner bootstrap error: %v\n", err)
		os.Exit(1)
	}
	cancelOwner()

	userRegistrar := user.NewRegistrar(mongoManager.Users(), logger)
	groupRegistrar := group.NewRegistrar(mongoManager.Groups(), logger)
	userRepository := domain.NewUserRepository(mongoManager.Users())
	statsProvider := store.NewStatsProvider(mongoManager.Users(), mongoManager.Groups())

	tgClient, err := telegram.NewClient(cfg, logger,
		telegram.WithUserRegistrar(userRegistrar),
		telegram.WithGroupRegistrar(groupRegistrar),
		telegram.WithMongoChecker(mongoManager),
		telegram.WithProcessStart(processStart),
		telegram.WithUserFetcher(userRepository),
		telegram.WithStatsProvider(statsProvider),
	)
	if err != nil {
		logger.WithError(err).Error("telegram client setup error")
		fmt.Fprintf(os.Stderr, "telegram client setup error: %v\n", err)
		os.Exit(1)
	}

	logger.WithField("event", "telegram_ready").Info("telegram client initialized")

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	telegramCtx, cancelTelegram := context.WithCancel(context.Background())
	tgDone := make(chan struct{})

	go func() {
		tgClient.Start(telegramCtx)
		close(tgDone)
	}()

	select {
	case <-signalCtx.Done():
		logger.WithField("event", "shutdown_signal").Info("received termination signal, stopping telegram polling")
	case <-tgDone:
		logger.WithField("event", "telegram_stopped_early").Warn("telegram client stopped before shutdown signal")
	}

	cancelTelegram()

	waitCtx, cancelWait := context.WithTimeout(context.Background(), telegramShutdownTimeout)
	select {
	case <-tgDone:
	case <-waitCtx.Done():
		logger.WithField("event", "telegram_shutdown_timeout").Warn("timed out waiting for telegram client to stop")
	}
	cancelWait()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), mongoDisconnectTimeout)
	if err := mongoManager.Close(shutdownCtx); err != nil {
		logger.WithError(err).Error("mongo disconnect error")
	} else {
		logger.WithField("event", "mongo_disconnect").Info("mongo client disconnected")
	}
	cancelShutdown()

	logger.WithField("event", "shutdown_complete").Info("shutdown complete")
}
