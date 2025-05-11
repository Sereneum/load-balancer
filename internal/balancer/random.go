package balancer

import (
	"math/rand/v2"
	"sync"
)

type Random struct {
	backends []string
	mu       sync.RWMutex
}

func NewRandom(backends []string) Balancer {
	return &Random{
		backends: append([]string(nil), backends...),
	}
}

func (r *Random) Next() (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.backends) == 0 {
		return "", ErrNoHealthyBackends
	}

	return r.backends[rand.IntN(len(r.backends))], nil
}

func (r *Random) Update(backends []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends = append([]string(nil), backends...)
}
