package ratelimiter

import (
	"encoding/json"
	"load-balancer/internal/apperror"
	"load-balancer/internal/utils/userkey"
	"log/slog"
	"net/http"
)

func Middleware(rl *Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cip, err := userkey.ReqToIP(r)
		if err != nil {
			slog.Info("Error parsing userkey-IP header")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(apperror.ErrUnauthorized.Code)
			json.NewEncoder(w).Encode(apperror.ErrUnauthorized)
			//http.Error(w, apperror.ErrUnauthorized.Message, apperror.ErrUnauthorized.Code)
			return
		}

		if !rl.Allow(cip.Value()) {
			slog.Info("Rate limit exceeded", slog.String("cip", cip.Value()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(apperror.ErrTooManyRequests.Code)
			json.NewEncoder(w).Encode(apperror.ErrTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
