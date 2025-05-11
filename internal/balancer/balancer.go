/*
Пакет balancer реализует:
- Интерфейсы и реализации стратегий балансировки
- Атомарную обертку для безопасного обновления стратегий
- Фабрику стратегий
*/

package balancer

import (
	"sync/atomic"
)

type Balancer interface {
	Next() (string, error)
	Update([]string)
}

// AtomicBalancer обеспечивает атомарную замену стратегий
type AtomicBalancer struct {
	value atomic.Value
}

func NewAtomicBalancer(initial Balancer) *AtomicBalancer {
	if initial == nil {
		panic("nil balancer")
	}

	ab := &AtomicBalancer{}
	ab.value.Store(initial)
	return ab
}

// Next делегирует вызов текущей стратегии
func (ab *AtomicBalancer) Next() (string, error) {
	return ab.Load().Next()
}

func (ab *AtomicBalancer) Update(backends []string) {
	ab.Load().Update(backends)
}

func (ab *AtomicBalancer) SetStrategy(newBalancer Balancer) {
	ab.Store(newBalancer)
}

func (ab *AtomicBalancer) Store(b Balancer) {
	if b == nil {
		panic("nil balancer")
	}
	ab.value.Store(b)
}

func (ab *AtomicBalancer) Load() Balancer {
	return ab.value.Load().(Balancer)
}
