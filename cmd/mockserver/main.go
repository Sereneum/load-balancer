package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"load-balancer/internal/config"
	"load-balancer/internal/prettylog"
	"load-balancer/internal/utils/userkey"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var configPath string

func init() {
	flag.StringVar(&configPath, "config", "", "Path to config")
}

type ServerState struct {
	mu                 sync.RWMutex
	isAlive            bool
	nextFailureTime    time.Time
	nextRecoveryTime   time.Time
	failureDuration    time.Duration
	failureProbability float32
	url                string
	port               string
}

func NewServerState(url, port string) *ServerState {
	return &ServerState{
		url:                url,
		port:               port,
		isAlive:            true,
		failureDuration:    10 * time.Second,
		failureProbability: 0.2, // 20% вероятность сбоя при проверке health
	}
}

func (s *ServerState) CheckHealth() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isAlive
}

func (s *ServerState) Update() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if s.isAlive {
		// Если сервер работает, проверяем не пора ли упасть
		if now.After(s.nextFailureTime) && rand.Float32() < s.failureProbability {
			s.isAlive = false
			s.nextRecoveryTime = now.Add(s.failureDuration)
			//log.Println("Server state changed to DOWN")
			// Warn для наглдности
			slog.Warn("Server state changed to DOWN", slog.String("URL", s.url))
		}
	} else {
		// Если сервер упал, проверяем не пора ли восстановиться
		if now.After(s.nextRecoveryTime) {
			s.isAlive = true
			s.nextFailureTime = now.Add(time.Duration(10+rand.IntN(20)) * time.Second)
			//log.Println("Server state changed to UP")
			slog.Info("Server state changed to UP", slog.String("URL", s.url))
		}
	}
}

// Глобальный список для хранения инстансов серверов для корректного Shutdown
var runningServers []*http.Server
var serverMutex sync.Mutex

func handler(port string, state *ServerState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// каким-то образом юзер прошел через rate-limit. white list либо еще что-то.
		cip, err := userkey.ReqToIP(r)
		if err != nil {
			slog.Error(err.Error())
		}
		attrUser := slog.String(cip.Type(), cip.Value())
		attrPort := slog.String("port", port)

		delay := time.Duration(rand.IntN(1000)+100) * time.Millisecond
		time.Sleep(delay)

		if !state.CheckHealth() {
			slog.Info(
				"Service unavailable",
				attrPort,
				attrUser,
			)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		if rand.Float32() < 0.1 { // 10% вероятность ошибки даже если сервер "живой"
			//log.Printf("[IP: %s] Simulated random failure (port %s)", ip, port)
			slog.Info(
				"Simulated random failure",
				attrPort,
				attrUser,
			)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		slog.Info(
			"Successful response",
			attrPort,
			attrUser,
		)
		_, _ = fmt.Fprintf(w, "Response from backend on port %s (delay: %v)", port, delay)
	}
}

func healthHandler(state *ServerState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state.Update()
		if state.CheckHealth() {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		}
	}
}

func startServer(ctx context.Context, port, serverUrl string, wg *sync.WaitGroup) {
	defer wg.Done()

	state := NewServerState(serverUrl, port)
	state.nextFailureTime = time.Now().Add(time.Duration(5+rand.IntN(10)) * time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler(port, state))
	mux.HandleFunc("/health", healthHandler(state))

	addr := ":" + port
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	serverMutex.Lock()
	runningServers = append(runningServers, server)
	serverMutex.Unlock()

	attrPort := slog.String("port", port)

	slog.Info("Starting mock server", attrPort)

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if !errors.Is(err, http.ErrServerClosed) {

				slog.Error(
					"Mock server exited with error",
					attrPort,
					slog.String("error", err.Error()),
				)
			} else {
				slog.Info(
					"Mock server shut down gracefully or closed",
					attrPort,
				)
			}
		}
	}()

	<-ctx.Done() // Ждем сигнала отмены из main mockserver
	slog.Info("Shutdown signal received for mock server instance", attrPort)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Mock server instance shutdown error",
			attrPort,
			slog.String("error", err.Error()))
	} else {
		slog.Info("Mock server instance stopped gracefully",
			attrPort)
	}

}

func main() {
	prettylog.InitLogger("debug")
	if err := config.Init(configPath); err != nil {
		slog.Error("config init error", slog.String("error", err.Error()))
		panic(err)
	}

	cfg := config.Get()

	// Контекст для управления жизненным циклом всех мок-серверов
	appCtx, appCancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup // Для ожидания завершения всех серверов

	for _, backend := range cfg.Backends {
		u, err := url.Parse(backend)
		if err != nil {
			slog.Error("Failed to parse backend URL for mock server", slog.String("URL", backend), slog.String("error", err.Error()))
			continue
		}
		wg.Add(1)
		go startServer(appCtx, u.Port(), backend, &wg)
	}

	// Ожидание сигнала для завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("Mock server received signal, shutting down...",
			slog.String("signal", sig.String()))
	case <-appCtx.Done(): // Если контекст отменили из другого места (маловероятно в этом сценарии)
		slog.Info("Mock server context cancelled, shutting down...")
	}

	// Инициируем отмену для всех запущенных инстансов мок-серверов
	appCancel()

	slog.Info("Waiting for all mock server instances to stop...")
	wg.Wait() // Ждем, пока все горутины startServer завершатся
	slog.Info("All mock server instances stopped. Mock server main exiting.")
}
