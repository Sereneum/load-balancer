package ratelimiter_test

import (
	"load-balancer/internal/config"
	"load-balancer/internal/ratelimiter"
	"load-balancer/internal/utils/userkey"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_Init(t *testing.T) {
	path := "../../configs/config.yaml"
	if err := config.Init(path); err != nil {
		t.Error(err)
	}

	cfg := config.Get()
	rlCfg := cfg.RateLimiter

	t.Log(rlCfg.ClientOverrides[0])
}

func Test_Allow(t *testing.T) {
	limiter := ratelimiter.NewLimiter(6, 1, nil)

	id := "id1"
	c := 0

	for i := 0; i < 10; i++ {
		if limiter.Allow(id) {
			c++
		}
	}

	if c != 6 {
		t.Errorf("Allow got %d, want %d", c, 6)
	}
}

func Test_Middleware(t *testing.T) {
	rl := ratelimiter.NewLimiter(1, 1, nil)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := ratelimiter.Middleware(rl, h)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "test-client")

	t.Log(userkey.ReqToIP(req))

	// Первый запрос должен пройти
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req)

	if w1.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w1.Code)
	}

	// Второй запрос — сразу, лимит превышен
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 Too Many Requests, got %d", w2.Code)
	}
}

func Test_MiddlewareOverClient(t *testing.T) {
	overClients := make(map[string]ratelimiter.ClientConfig)

	var (
		testuser  = "test-user"
		superuser = "superuser"
		ddoser    = "ddoser"
		key       = "X-Forwarded-For"
	)

	overClients[superuser] = ratelimiter.ClientConfig{
		Capacity: 10,
		Rate:     1,
	}

	overClients[ddoser] = ratelimiter.ClientConfig{
		Capacity: 1,
		Rate:     1,
	}

	rl := ratelimiter.NewLimiter(2, 1, overClients)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := ratelimiter.Middleware(rl, h)

	t.Run(testuser, func(t *testing.T) {
		t.Log(testuser)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(key, testuser)

		t.Log(userkey.ReqToIP(req))

		for i := 0; i < 3; i++ {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if i < 2 {
				if w.Code != http.StatusOK {
					t.Errorf("expected 200 OK, got %d", w.Code)
				}
			} else {
				if w.Code != http.StatusTooManyRequests {
					t.Errorf("expected 429, got %d", w.Code)
				}
			}
		}
	})

	t.Run(superuser, func(t *testing.T) {
		t.Log(superuser)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(key, superuser)

		t.Log(userkey.ReqToIP(req))

		for i := 0; i < 5; i++ {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected 200 OK, got %d", w.Code)
			}
		}
	})

	t.Run(ddoser, func(t *testing.T) {
		t.Log(ddoser)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(key, ddoser)

		t.Log(userkey.ReqToIP(req))

		for i := 0; i < 2; i++ {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if i < 1 {
				if w.Code != http.StatusOK {
					t.Errorf("expected 200 OK, got %d", w.Code)
				}
			} else {
				if w.Code != http.StatusTooManyRequests {
					t.Errorf("expected 429, got %d", w.Code)
				}
			}
		}
	})
}
