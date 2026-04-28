package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michibiki-io/pma-gateway/backend/internal/auditmeta"
	"github.com/michibiki-io/pma-gateway/backend/internal/buildinfo"
	"github.com/michibiki-io/pma-gateway/backend/internal/config"
	pmacrypto "github.com/michibiki-io/pma-gateway/backend/internal/crypto"
	"github.com/michibiki-io/pma-gateway/backend/internal/httpserver"
	"github.com/michibiki-io/pma-gateway/backend/internal/logging"
	"github.com/michibiki-io/pma-gateway/backend/internal/storage"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	ctx := context.Background()
	if cfg.DatabaseDriver == "sqlite" && cfg.DataDir != "" {
		if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
			logger.Fatal("create data directory", zap.Error(err))
		}
	}

	cipher, err := pmacrypto.NewCipher(cfg.MasterKey)
	if err != nil {
		logger.Fatal("initialize credential cipher", zap.Error(err))
	}

	store, err := storage.Open(ctx, storage.Options{
		Driver: cfg.DatabaseDriver,
		Path:   cfg.DatabasePath,
		DSN:    cfg.DatabaseDSN,
	}, cipher)
	if err != nil {
		logger.Fatal("open storage", zap.Error(err))
	}
	defer func() { _ = store.Close() }()

	if err := store.Migrate(ctx); err != nil {
		logger.Fatal("run migrations", zap.Error(err))
	}

	if cfg.Bootstrap.Enabled {
		result, err := store.ApplyBootstrap(ctx, cfg.Bootstrap)
		if err != nil {
			_ = store.InsertAuditEvent(ctx, storage.AuditEvent{
				Actor:      "system",
				Action:     auditmeta.ActionBootstrapApply,
				TargetType: auditmeta.TargetTypeSystem,
				Result:     "failure",
				Message:    "Bootstrap import failed",
				Metadata:   map[string]any{"error": err.Error()},
			})
			logger.Fatal("apply bootstrap configuration", zap.Error(err))
		}
		if result.Applied {
			_ = store.InsertAuditEvent(ctx, storage.AuditEvent{
				Actor:      "system",
				Action:     auditmeta.ActionBootstrapApply,
				TargetType: auditmeta.TargetTypeSystem,
				Result:     "success",
				Message:    "Bootstrap import applied",
				Metadata: map[string]any{
					"mode":        cfg.Bootstrap.Mode,
					"credentials": result.Credentials,
					"mappings":    result.Mappings,
				},
			})
		}
	}

	router := httpserver.NewRouter(cfg, store, logger)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		version := buildinfo.Current()
		logger.Info(
			"starting pma-gateway backend",
			zap.String("listen", cfg.ListenAddr),
			zap.String("version", version.AppDisplayVersion),
			zap.String("commit", version.AppCommit),
			zap.String("phpMyAdminVersion", version.PHPMyAdminVersion),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
	case err := <-errCh:
		logger.Fatal("backend server failed", zap.Error(err))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("graceful shutdown failed", zap.Error(err))
	}
	logger.Info("backend stopped")
}
