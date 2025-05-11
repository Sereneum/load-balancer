package ratelimiter

import (
	"sync"
	"time"
)

type Bucket struct {
	Capacity     int
	Tokens       int
	RefillRate   int // tokens per second
	LastRefilled time.Time
	mu           sync.Mutex
}

func (b *Bucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.LastRefilled).Seconds()
	newTokens := int(elapsed * float64(b.RefillRate))

	if newTokens > 0 {
		b.Tokens = min(b.Capacity, b.Tokens+newTokens)
		b.LastRefilled = now
	}

	if b.Tokens > 0 {
		b.Tokens--
		return true
	}

	return false
}
