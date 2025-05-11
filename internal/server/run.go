package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Run(appCtx context.Context, appCancel context.CancelFunc, s *http.Server) {

	serverErrChan := make(chan error, 1)
	//go func() {
	slog.Info("HTTP server starting", slog.String("address", s.Addr))
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("HTTP server ListenAndServe error", slog.String("error", err.Error()))
		go func() {
			serverErrChan <- err
			close(serverErrChan)
		}()
	}

	//}()

	//go func() {
	//slog.Info("server started", slog.String("addr", s.Addr))
	//if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
	//	slog.Error(err.Error())
	//	panic(err)
	//}
	//}()

	gracefulShutdown(appCtx, appCancel, s, serverErrChan)
}

func gracefulShutdown(appCtx context.Context, appCancel context.CancelFunc, s *http.Server, serverErrChan chan error) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrChan:
		if err != nil {
			slog.Error(
				"Failed to start or run HTTP server, initiating application shutdown.",
				slog.String("error", err.Error()))
		}
	case sig := <-quit:
		slog.Info("Shutdown signal received", slog.String("signal", sig.String()))
	case <-appCtx.Done():
		slog.Info("Application context cancelled")
	}

	// Инициируем отмену для всех компонентов, которые слушают appCtx
	slog.Info("Broadcasting shutdown signal to all components...")
	appCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := s.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", slog.String("error", err.Error()))
	} else {
		slog.Info("HTTP server gracefully stopped.")
	}

	slog.Info("server exiting")
}
