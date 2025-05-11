package balancer

import (
	"sync"
	"testing"
)

func TestRoundRobinConcurrency(t *testing.T) {
	rr := NewRoundRobin([]string{"a", "b", "c"})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr.Next()
			rr.Update([]string{"a", "b"})
		}()
	}
	wg.Wait()
}
