package apperror

import "net/http"

type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *AppError) Error() string {
	return e.Message
}

func New(message string, code int) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// Готовые ошибки
var (
	ErrTooManyRequests           = New("Too many requests", http.StatusTooManyRequests)
	ErrUnauthorized              = New("Unable to identify user", http.StatusUnauthorized)
	ErrNoBackendAvailable        = New("No backend available", http.StatusServiceUnavailable)
	ErrStatusBadGateway          = New("Bad Gateway", http.StatusBadGateway)
	ErrStatusInternalServerError = New("Internal Server Error", http.StatusInternalServerError)
)
