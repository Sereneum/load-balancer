package config_test

import (
	"fmt"
	"load-balancer/internal/config"
	"testing"
	"time"
)

func Test_Init(t *testing.T) {
	path := "../../configs/config.yaml"

	err := config.Init(path)
	if err != nil {
		t.Fatal("config init error:", err)
	}

	config.Subscribe(func(c *config.Config) {
		fmt.Println("[Subscriber] Updated RateLimiter:", c.RateLimiter)
	})

	for {
		cfg := config.Get()
		fmt.Printf("Rate limiter: default_capacity=%d | default_rate_per_second=%f\n",
			cfg.RateLimiter.DefaultCapacity, cfg.RateLimiter.DefaultRate)
		time.Sleep(5 * time.Second)
	}
}
