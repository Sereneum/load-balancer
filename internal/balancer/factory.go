package balancer

import (
	"errors"
	"log/slog"
)

var ErrNoHealthyBackends = errors.New("no healthy backends available")

type StrategyFactory interface {
	Create(strategy string, backends []string) Balancer
}

type defaultStrategyFactory struct{}

func NewStrategyFactory() StrategyFactory {
	return &defaultStrategyFactory{}
}

func (f *defaultStrategyFactory) Create(strategy string, backends []string) Balancer {
	switch strategy {
	case "round-robin":
		return NewRoundRobin(backends)
	case "random":
		return NewRandom(backends)
	default:
		slog.Warn("unknown strategy, using round-robin", slog.String("strategy", strategy))
		return NewRoundRobin(backends)
	}
}
func NewBalancer(strategy string, backends []string) Balancer {
	switch strategy {
	case "round-robin":
		return NewRoundRobin(backends)
	default:
		slog.Warn("unknown strategy", slog.String("strategy", strategy))
		return NewRoundRobin(backends)
	}
}
