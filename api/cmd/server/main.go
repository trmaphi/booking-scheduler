package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"scheduler/api/internal/application"
	"scheduler/api/internal/config"
	"scheduler/api/internal/httpapi"
	"scheduler/api/internal/postgres"
	"scheduler/api/internal/telemetry"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("cannot start API", "reason", "invalid configuration")
		os.Exit(1)
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Error("cannot start API", "error", "invalid DATABASE_URL")
		os.Exit(1)
	}
	poolConfig.MaxConns = 5
	poolConfig.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		logger.Error("cannot start API", "error", "database pool initialization failed")
		os.Exit(1)
	}
	defer pool.Close()

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           newHandlerWithTelemetry(pool, cfg.AllowedOrigin, telemetry.NewJSONRecorder(os.Stdout)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		logger.Error("API server failed", "reason", "listener unavailable")
		os.Exit(1)
	}
	defer listener.Close()

	logger.Info("API listening", "address", cfg.ListenAddress)
	if err := serve(ctx, server, listener); err != nil {
		logger.Error("API server failed", "reason", "server stopped unexpectedly")
		os.Exit(1)
	}
}

func newHandler(pool *pgxpool.Pool, allowedOrigin string) http.Handler {
	return newHandlerWithTelemetry(pool, allowedOrigin, telemetry.Nop())
}

func newHandlerWithTelemetry(pool *pgxpool.Pool, allowedOrigin string, recorder telemetry.Recorder) http.Handler {
	recorder = telemetry.Safe(recorder)
	repository := postgres.NewRepository(pool, recorder)
	availability := application.NewAvailabilityService(repository)
	return httpapi.NewRouter(httpapi.Dependencies{
		Readiness:          pool.Ping,
		BookingOptions:     repository.BookingOptions,
		AvailableSlots:     availability.AvailableSlots,
		ConfirmAppointment: repository.Confirm,
		Appointments:       repository.Appointments,
		AppointmentByID:    repository.AppointmentByID,
		AllowedOrigin:      allowedOrigin,
		Telemetry:          recorder,
	})
}

func serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()

	select {
	case err := <-serveDone:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-serveDone
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
