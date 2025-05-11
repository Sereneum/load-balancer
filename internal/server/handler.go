/*
Пакет server реализует:
- Проксирование запросов к бэкендам
- Обработку ошибок балансировки
- Формирование структурированных логов
- JSON-ответы при ошибках
*/

package server

import (
	"encoding/json"
	"errors"
	"load-balancer/internal/apperror"
	"load-balancer/internal/balancer"
	"load-balancer/internal/proxy"
	"load-balancer/internal/utils/userkey"
	"log/slog"
	"net/http"
	"net/url"
)

type Handler struct {
	balancer balancer.Balancer
}

func NewHandler(b balancer.Balancer) *Handler {
	return &Handler{balancer: b}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// индикация пользователя (userkey-IP)
	cip, _ := userkey.ReqToIP(r)
	attr := slog.String(cip.Type(), cip.Value())
	slog.Info("Request", attr)

	backend, err := h.balancer.Next()

	if errors.Is(err, balancer.ErrNoHealthyBackends) {
		slog.Error("No backend available", attr)
		jsonError(w, apperror.ErrNoBackendAvailable)
		return
	}

	if err != nil {
		slog.Error("Balancer error", slog.String("error", err.Error()), attr)
		jsonError(w, apperror.ErrStatusInternalServerError)
		return
	}

	targetURL, err := url.Parse(backend)
	if err != nil {
		slog.Error(
			"Invalid backend URL",
			slog.String("url", backend),
			slog.String("error", err.Error()),
			attr,
		)
		jsonError(w, apperror.ErrStatusInternalServerError)
		return
	}

	slog.Info("Backend available", slog.String("server_url", targetURL.String()), attr)
	p := proxy.NewReverseProxy(targetURL.String())
	p.ErrorHandler = h.proxyErrorHandler(targetURL.String())
	p.ServeHTTP(w, r)
}

func (h *Handler) proxyErrorHandler(backend string) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("Error proxying request", slog.String("error", err.Error()))
		jsonError(w, apperror.ErrStatusBadGateway)
	}
}

func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func jsonError(w http.ResponseWriter, appError *apperror.AppError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appError.Code)
	json.NewEncoder(w).Encode(appError.Message)
}
