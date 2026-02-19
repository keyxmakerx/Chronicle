// Package main is the entry point for the Chronicle server. It loads
// configuration, establishes database connections, wires together all
// plugins/modules/widgets, and starts the HTTP server.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/keyxmakerx/chronicle/internal/app"
	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/database"
)

func main() {
	// --- Load Configuration ---
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	// Configure structured logging based on environment.
	setupLogging(cfg)

	slog.Info("starting Chronicle",
		slog.String("env", cfg.Env),
		slog.Int("port", cfg.Port),
	)

	// --- Connect to MariaDB ---
	db, err := database.NewMariaDB(cfg.Database)
	if err != nil {
		slog.Error("failed to connect to MariaDB", slog.Any("error", err))
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("connected to MariaDB")

	// --- Connect to Redis ---
	rdb, err := database.NewRedis(cfg.Redis)
	if err != nil {
		slog.Error("failed to connect to Redis", slog.Any("error", err))
		os.Exit(1)
	}
	defer rdb.Close()
	slog.Info("connected to Redis")

	// --- Create Application ---
	application := app.New(cfg, db, rdb)

	// Register all routes (public, plugin, module, widget, API).
	application.RegisterRoutes()

	// --- Graceful Shutdown ---
	// Listen for interrupt/term signals to drain connections cleanly.
	// This is required for Docker/Cosmos restarts to be seamless.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		slog.Info("shutting down server...")

		// Give in-flight requests 10 seconds to complete.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := application.Echo.Shutdown(ctx); err != nil {
			slog.Error("server forced shutdown", slog.Any("error", err))
		}
	}()

	// --- Start Server ---
	if err := application.Start(); err != nil {
		// Echo returns http.ErrServerClosed on graceful shutdown, which is expected.
		slog.Info("server stopped", slog.Any("reason", err))
	}
}

// setupLogging configures the global slog logger based on the environment.
// Development uses text format for readability. Production uses JSON for
// structured log aggregation.
func setupLogging(cfg *config.Config) {
	var handler slog.Handler

	if cfg.IsDevelopment() {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}

	slog.SetDefault(slog.New(handler))
}
