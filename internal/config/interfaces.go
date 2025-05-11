package config

import "time"

type Config struct {
	Server      ServerSettings    `yaml:"server"`
	Strategy    string            `yaml:"strategy"`
	Backends    []string          `yaml:"backends"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
	RateLimiter RateLimiterConfig `yaml:"rate_limiter"`
	LogFile     string            `yaml:"log_file"`  // Путь к файлу логов
	LogLevel    string            `yaml:"log_level"` // e.g., "debug", "info", "error"
}

type ServerSettings struct {
	Port         string        `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type HealthCheckConfig struct {
	IntervalSeconds time.Duration `yaml:"interval_seconds"`
	TimeoutSeconds  time.Duration `yaml:"timeout_seconds"`
	Path            string        `yaml:"path"` // Path for health check, e.g. /health
}

type RateLimiterConfig struct {
	Enabled         bool                    `yaml:"enabled"`
	DefaultCapacity int                     `yaml:"default_capacity"`
	DefaultRate     int                     `yaml:"default_rate_per_second"`
	ClientOverrides []ClientRateLimitConfig `yaml:"client_overrides"`
}

type ClientRateLimitConfig struct {
	ClientID string `yaml:"client_id"`
	Capacity int    `yaml:"capacity"`
	Rate     int    `yaml:"rate_per_second"`
}
