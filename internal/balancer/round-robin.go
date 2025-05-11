package balancer

import (
	"sync"
)

type RoundRobin struct {
	backends []string
	index    int
	mu       sync.Mutex
}

func NewRoundRobin(backends []string) Balancer {
	return &RoundRobin{backends: backends}
}

func (r *RoundRobin) Update(backends []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Сбрасываем индекс при изменении списка
	if !slicesEqual(r.backends, backends) {
		r.index = 0
	}

	r.backends = append([]string(nil), backends...)
	if len(r.backends) > 0 && r.index >= len(r.backends) {
		r.index = 0
	}
}

func (r *RoundRobin) Next() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.backends) == 0 {
		return "", ErrNoHealthyBackends
	}

	backend := r.backends[r.index]
	r.index = (r.index + 1) % len(r.backends)

	return backend, nil
}

// slicesEqual Вспомогательная функция для сравнения слайсов
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
