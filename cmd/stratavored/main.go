package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/meridian-lex/stratavore/internal/daemon"
	"github.com/meridian-lex/stratavore/internal/messaging"
	"github.com/meridian-lex/stratavore/internal/notifications"
	"github.com/meridian-lex/stratavore/internal/observability"
	"github.com/meridian-lex/stratavore/internal/session"
	"github.com/meridian-lex/stratavore/internal/storage"
	"github.com/meridian-lex/stratavore/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Version   = "1.4.0"
	BuildTime = "unknown"
	Commit    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Setup logger
	logger, err := setupLogger(cfg.Observability.LogLevel, cfg.Observability.LogFormat)
	if err != nil {
		return fmt.Errorf("setup logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("starting stratavore daemon",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("commit", Commit))

	// Write PID file so 'stratavore resume' can detect us even without tmux.
	if err := session.WritePIDFile(); err != nil {
		logger.Warn("failed to write PID file", zap.Error(err))
	}
	defer func() {
		if err := session.DeleteDaemonSession(); err != nil {
			logger.Warn("failed to clean up session files on exit", zap.Error(err))
		}
	}()

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to PostgreSQL
	logger.Info("connecting to postgresql",
		zap.String("host", cfg.Database.PostgreSQL.Host),
		zap.Int("port", cfg.Database.PostgreSQL.Port))

	db, err := storage.NewPostgresClient(
		ctx,
		cfg.Database.PostgreSQL.GetConnectionString(),
		cfg.Database.PostgreSQL.MaxConns,
		cfg.Database.PostgreSQL.MinConns,
	)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	logger.Info("connected to postgresql")

	// Connect to RabbitMQ
	logger.Info("connecting to rabbitmq",
		zap.String("host", cfg.Docker.RabbitMQ.Host),
		zap.Int("port", cfg.Docker.RabbitMQ.Port))

	mqClient, err := messaging.NewClient(messaging.Config{
		Host:              cfg.Docker.RabbitMQ.Host,
		Port:              cfg.Docker.RabbitMQ.Port,
		User:              cfg.Docker.RabbitMQ.User,
		Password:          cfg.Docker.RabbitMQ.Password,
		Exchange:          cfg.Docker.RabbitMQ.Exchange,
		PublisherConfirms: cfg.Docker.RabbitMQ.PublisherConfirms,
	}, logger)
	if err != nil {
		return fmt.Errorf("connect to rabbitmq: %w", err)
	}
	defer mqClient.Close()

	logger.Info("connected to rabbitmq")

	// Declare queues
	if err := mqClient.DeclareQueue("stratavore.daemon.events", []string{"#"}); err != nil {
		logger.Error("failed to declare queue", zap.Error(err))
	}

	// Initialize Telegram notifications
	var notifier *notifications.Client
	if cfg.Docker.Telegram.Token != "" && cfg.Docker.Telegram.ChatID != "" {
		notifier = notifications.NewClient(notifications.Config{
			Token:  cfg.Docker.Telegram.Token,
			ChatID: cfg.Docker.Telegram.ChatID,
		}, logger)

		hostname, _ := os.Hostname()
		notifier.DaemonStarted(Version, hostname)
		logger.Info("telegram notifications enabled")
	} else {
		logger.Warn("telegram notifications disabled (no token/chat_id configured)")
	}

	// Create runner manager
	runnerMgr := daemon.NewRunnerManager(db, mqClient, logger)

	// Create API handler
	apiHandler := daemon.NewGRPCServer(runnerMgr, db, logger, cfg.Daemon.Port_GRPC)

	// Start HTTP API server
	httpServer := daemon.NewHTTPServer(cfg.Daemon.Port_HTTP, apiHandler, logger, &cfg.Security)
	go func() {
		if err := httpServer.Start(); err != nil {
			logger.Error("HTTP API server error", zap.Error(err))
		}
	}()

	// Start outbox publisher
	outboxPublisher := messaging.NewOutboxPublisher(
		db,
		mqClient,
		time.Duration(cfg.Daemon.OutboxPollInterval)*time.Second,
		50, // batch size
		logger,
	)
	go outboxPublisher.Start(ctx)

	// Start reconciliation loop
	go startReconciliationLoop(ctx, runnerMgr, cfg.Daemon.ReconcileInterval, logger)

	// Start metrics server
	var metricsServer *observability.MetricsServer
	if cfg.Docker.Prometheus.Enabled {
		metricsServer = observability.NewMetricsServer(cfg.Docker.Prometheus.Port, logger)
		go func() {
			if err := metricsServer.Start(); err != nil {
				logger.Error("metrics server error", zap.Error(err))
			}
		}()

		// Update metrics periodically
		go startMetricsUpdateLoop(ctx, metricsServer, runnerMgr, logger)
	}

	// Start gRPC server
	grpcServer := daemon.NewGRPCServer(runnerMgr, db, logger, cfg.Daemon.Port_GRPC)
	go func() {
		if err := grpcServer.Start(); err != nil {
			logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	logger.Info("stratavore daemon started successfully",
		zap.Int("grpc_port", cfg.Daemon.Port_GRPC),
		zap.Int("metrics_port", cfg.Docker.Prometheus.Port))

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	// Send shutdown notification if notifier is configured
	if notifier != nil {
		hostname, _ := os.Hostname()
		notifier.DaemonStopped(hostname)
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		time.Duration(cfg.Daemon.ShutdownTimeout)*time.Second,
	)
	defer shutdownCancel()

	logger.Info("shutting down daemon...")

	// Stop HTTP server
	httpServer.Stop(shutdownCtx)

	// Stop metrics server
	if metricsServer != nil {
		metricsServer.Stop()
	}

	// Stop outbox publisher
	outboxPublisher.Stop()

	// Shutdown runner manager
	if err := runnerMgr.Shutdown(shutdownCtx); err != nil {
		logger.Error("error during shutdown", zap.Error(err))
	}

	logger.Info("daemon shutdown complete")
	return nil
}

func setupLogger(level, format string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	var cfg zap.Config
	if format == "json" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}

	cfg.Level = zap.NewAtomicLevelAt(zapLevel)
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	return cfg.Build()
}

func startReconciliationLoop(ctx context.Context, mgr *daemon.RunnerManager, intervalSeconds int, logger *zap.Logger) {
	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	logger.Info("reconciliation loop started", zap.Int("interval_seconds", intervalSeconds))

	for {
		select {
		case <-ticker.C:
			if err := mgr.ReconcileRunners(ctx); err != nil {
				logger.Error("reconciliation error", zap.Error(err))
			}
		case <-ctx.Done():
			logger.Info("reconciliation loop stopped")
			return
		}
	}
}

func startMetricsUpdateLoop(ctx context.Context, metrics *observability.MetricsServer, mgr *daemon.RunnerManager, logger *zap.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ticker.C:
			runners := mgr.GetActiveRunners()
			metrics.UpdateRunnerMetrics(runners)
			metrics.UpdateDaemonUptime(time.Since(startTime).Seconds())
		case <-ctx.Done():
			return
		}
	}
}
