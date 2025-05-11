/*
Пакет ratelimiter реализует:
- Token bucket алгоритм
- Клиент-специфичные лимиты
- Фоновую очистку неактивных бакетов
- Атомарное обновление конфигурации
*/

package ratelimiter

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type ClientConfig struct {
	Capacity int
	Rate     int
}

type RateLimiter interface {
	Allow(string) bool
}

type Limiter struct {
	buckets         map[string]*Bucket
	lastSeen        map[string]time.Time
	overrideClients map[string]ClientConfig // Для специфичных настроек клиентов

	defaultCapacity int
	defaultRate     int
	mu              sync.RWMutex

	// Для управления циклом очистки
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
	cleanupWg     sync.WaitGroup
}

func NewLimiter(
	defaultCap, defaultRate int,
	overClientsList map[string]ClientConfig,
) *Limiter {
	return &Limiter{
		buckets:         make(map[string]*Bucket),
		lastSeen:        make(map[string]time.Time),
		overrideClients: overClientsList,
		defaultCapacity: defaultCap,
		defaultRate:     defaultRate,
	}
}

// UpdateConfig обновляет конфигурацию Rate Limiter.
// Изменения затронут новые бакеты или бакеты, которые будут пересозданы.
func (l *Limiter) UpdateConfig(newDefaultCapacity, newDefaultRate int, newOverrides map[string]ClientConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()

	slog.Info("RateLimiter: updating configuration",
		slog.Int("new_default_capacity", newDefaultCapacity),
		slog.Int("new_default_rate", newDefaultRate))

	l.defaultCapacity = newDefaultCapacity
	l.defaultRate = newDefaultRate
	l.overrideClients = newOverrides
	// TODO обновить buckets для newOverrides
}

func (l *Limiter) getBucket(clientID string) *Bucket {
	l.mu.RLock() // +rl
	//defer l.mu.Unlock() // -l
	b, exists := l.buckets[clientID]

	currentDefaultCapacity := l.defaultCapacity
	currentDefaultRate := l.defaultRate
	clientSpecificConfig, hasOverride := l.overrideClients[clientID]
	l.mu.RUnlock() // +rl

	if exists {
		l.mu.Lock()
		l.lastSeen[clientID] = time.Now()
		l.mu.Unlock()
		return b
	}

	l.mu.Lock()         // +l
	defer l.mu.Unlock() // -l

	// Двойная проверка (Double-Checked Locking), на случай если бакет был создан другим потоком
	// пока мы ждали полную блокировку.
	if b, exists = l.buckets[clientID]; exists {
		l.lastSeen[clientID] = time.Now()
		return b
	}

	// Определяем параметры для нового бакета
	capacity := currentDefaultCapacity
	rate := currentDefaultRate
	if hasOverride {
		capacity = clientSpecificConfig.Capacity
		rate = clientSpecificConfig.Rate
	}

	newBucket := &Bucket{
		Capacity:     capacity,
		Tokens:       capacity,
		RefillRate:   rate,
		LastRefilled: time.Now(),
	}

	l.buckets[clientID] = newBucket
	l.lastSeen[clientID] = time.Now()

	slog.Debug(
		"RateLimiter: created new bucket",
		slog.String("client_id", clientID),
		slog.Int("capacity", capacity),
		slog.Int("rate", rate))
	return newBucket
}

func (l *Limiter) Allow(clientID string) bool {
	b := l.getBucket(clientID)
	return b.allow()
}

func (l *Limiter) StartCleanup(parentCtx context.Context, interval time.Duration, ttl time.Duration) {

	l.mu.Lock()
	if l.cleanupCancel != nil {
		slog.Info(
			"RateLimiter: Cleanup process already running or stopping. Ignoring StartCleanup call.")
		l.mu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(parentCtx)
	l.cleanupCtx = ctx
	l.cleanupCancel = cancel

	l.mu.Unlock()

	l.cleanupWg.Add(1)
	go func() {
		defer l.cleanupWg.Done()
		slog.Info(
			"RateLimiter: cleanup goroutine started.",
			slog.Duration("interval", interval),
			slog.Duration("ttl", ttl))

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("RateLimiter: cleanup goroutine stopping.")
				return
			case <-ticker.C:
				l.mu.Lock()
				now := time.Now()
				cleanedCount := 0
				for cid, b := range l.buckets {
					lastSeen, ok := l.lastSeen[cid]
					if ok && now.Sub(lastSeen) > ttl && b.Tokens == b.Capacity {
						delete(l.buckets, cid)
						delete(l.lastSeen, cid)
						cleanedCount++
					}
				}
				if cleanedCount > 0 {
					slog.Debug(
						"RateLimiter: cleaned up inactive buckets",
						slog.Int("count", cleanedCount))
				}
				l.mu.Unlock()
			}
		}
	}()
}

func (l *Limiter) StopCleanup() {
	l.mu.Lock()
	if l.cleanupCancel != nil {
		slog.Info("RateLimiter: StopCleanup called, signaling cleanup goroutine to terminate.")
		l.cleanupCancel()
		l.cleanupCancel = nil
	} else {
		slog.Info("RateLimiter: StopCleanup called, but no active cleanup goroutine to stop.")
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
	l.cleanupWg.Wait()
	slog.Info("RateLimiter: cleanup goroutine gracefully stopped.")
}
