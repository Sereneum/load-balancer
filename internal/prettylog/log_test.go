package prettylog_test

import (
	"load-balancer/internal/prettylog"
	"log/slog"
	"testing"
)

func TestLogger_Init(t *testing.T) {

	prettylog.InitLogger("debug")
	
	slog.Debug(
		"executing database query",
		slog.String("query", "SELECT * FROM users"),
	)
	slog.Info("image upload successful", slog.String("image_id", "39ud88"))
	slog.Warn(
		"storage is 90% full",
		slog.String("available_space", "900.1 MB"),
	)
	slog.Error(
		"An error occurred while processing the request",
		slog.String("url", "https://example.com"),
	)
}
