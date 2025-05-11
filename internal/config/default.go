package config

import "time"

type option func(*Config)

func withDefaultStrategy() option {
	return func(cfg *Config) {
		if cfg.Strategy == "" {
			cfg.Strategy = "round-robin"
		}
	}
}

func withDefaultServer() option {
	return func(cfg *Config) {
		if cfg.Server.Port == "" {
			cfg.Server.Port = "8080"
		}
		if cfg.Server.ReadTimeout == 0 {
			cfg.Server.ReadTimeout = 5 * time.Second
		}
		if cfg.Server.WriteTimeout == 0 {
			cfg.Server.WriteTimeout = 10 * time.Second
		}
	}
}

func withDefaultHealthCheck() option {
	return func(cfg *Config) {
		if cfg.HealthCheck.IntervalSeconds == 0 {
			cfg.HealthCheck.IntervalSeconds = 10 * time.Second
		}
		if cfg.HealthCheck.TimeoutSeconds == 0 {
			cfg.HealthCheck.TimeoutSeconds = 5 * time.Second
		}
		if cfg.HealthCheck.Path == "" {
			cfg.HealthCheck.Path = "/health"
		}
	}
}

func withDefaultRateLimiter() option {
	return func(cfg *Config) {
		if cfg.RateLimiter.DefaultRate == 0 {
			cfg.RateLimiter.DefaultRate = 10
		}
		if cfg.RateLimiter.DefaultCapacity == 0 {
			cfg.RateLimiter.DefaultCapacity = 100
		}
	}
}

func useDefault(cfg *Config, options ...option) {
	for _, op := range options {
		op(cfg)
	}
}

func loadDefaultValues(cfg *Config) {
	useDefault(
		cfg,
		withDefaultServer(),
		withDefaultHealthCheck(),
		withDefaultStrategy(),
		withDefaultRateLimiter(),
	)
}
