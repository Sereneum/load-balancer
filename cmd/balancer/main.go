/*
Пакет main - точка входа для балансировщика нагрузки. Основные компоненты:
- Загрузка конфигурации
- Инициализация логгера
- Создание и управление основными компонентами системы
- Запуск HTTP сервера
- Обработка graceful shutdown
*/

package main

import (
	"context"
	"flag"
	"load-balancer/internal/balancer"
	"load-balancer/internal/config"
	"load-balancer/internal/health"
	"load-balancer/internal/prettylog"
	"load-balancer/internal/ratelimiter"
	"load-balancer/internal/server"
	"log"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

var configPath string

func init() {
	flag.StringVar(&configPath, "config", "", "Path to config")
}

func main() {
	// --- INIT CONFIG ---
	// Загружаем конфигурацию. Если ошибка - паникуем
	cfg := loadConfig()

	// --- LOGGER ---
	prettylog.InitLogger(cfg.LogLevel) // TODO: Уровень из конфига
	slog.Info("config initialized")
	slog.Info("logger initialized", slog.String("level", cfg.LogLevel))
	slog.Info("Application starting...")

	// --- APP CONTEXT ---
	// Контекст для управления жизненным циклом всего приложения
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()
	var appWg sync.WaitGroup // WaitGroup для ожидания завершения всех компонентов

	// --- BALANCER ---
	// Balancer обновляется через HealthChecker's OnUpdate callback списком живых серверов.
	// Стратегия пока не обновляется "на лету".
	b := setupBalancer(cfg)

	// --- RATE LIMITER ---
	rl := setupRateLimiter(appCtx, cfg, &appWg)

	// --- HEALTH CHECKER ---
	setupAndRunHealthChecker(appCtx, &appWg, cfg, b)

	// --- HANDLER ---
	h := setupHandler(b)

	// --- HTTP SERVER ---
	s := setupHttpServer(cfg, h, rl) // rl передается для middleware

	// --- RUN SERVER + GRACEFUL SHUTDOWN ---
	server.Run(appCtx, appCancel, s)

	// Ожидаем завершения всех фоновых горутин, управляемых appWg
	//slog.Info("Waiting for all application components to stop...")
	appWg.Wait()
	slog.Info("All application components stopped. Exiting.")
}

func loadConfig() *config.Config {
	if err := config.Init(configPath); err != nil {
		log.Fatal("config init error", err.Error())
	}

	return config.Get()
}

func setupBalancer(cfg *config.Config) balancer.Balancer {
	factory := balancer.NewStrategyFactory()
	b := factory.Create(cfg.Strategy, cfg.Backends)
	ab := balancer.NewAtomicBalancer(b)

	slog.Info("balancer initialized", slog.String("strategy", cfg.Strategy))

	config.Subscribe(func(newCfg *config.Config) {
		// 3. Обновить стратегию балансировщика (если она изменилась)
		// Если стратегия меняется, возможно, нужно пересоздать объект балансировщика.
		// И обновить ссылку на него в server.Handler.
		// А также callback в HealthChecker, т.к. он захватывал 'b' по значению.
		// TODO
		if newCfg.Strategy != cfg.Strategy {
			newBalancer := factory.Create(newCfg.Strategy, newCfg.Backends)
			ab.SetStrategy(newBalancer) // Атомарная замена
		}

		// Обновление серверов не нежно, т.к. Health Checker
		// подхватывает это изменение и сообщает балансировщику
	})

	return ab
}

// setupRateLimiter создает/настраивает распределенный ограничитель запросов
func setupRateLimiter(appCtx context.Context, cfg *config.Config, appWg *sync.WaitGroup) *ratelimiter.Limiter {
	// cfg.RateLimiter.ClientOverrides -> ...ratelimiter.ClientConfig
	getOverClients := func(cfg *config.Config) map[string]ratelimiter.ClientConfig {
		clientMap := make(map[string]ratelimiter.ClientConfig)
		for _, client := range cfg.RateLimiter.ClientOverrides {
			clientMap[client.ClientID] = ratelimiter.ClientConfig{
				Capacity: client.Capacity,
				Rate:     client.Rate,
			}
		}
		return clientMap
	}

	rl := ratelimiter.NewLimiter(
		cfg.RateLimiter.DefaultCapacity,
		cfg.RateLimiter.DefaultRate,
		getOverClients(cfg),
	)
	// TODO: Сделать интервал и TTL очистки конфигурируемыми
	appWg.Add(1)
	go func() {
		defer appWg.Done()
		rl.StartCleanup(appCtx, 3*time.Minute, 5*time.Minute)
	}()

	config.Subscribe(func(newCfg *config.Config) {
		rl.UpdateConfig(
			newCfg.RateLimiter.DefaultCapacity,
			newCfg.RateLimiter.DefaultRate,
			getOverClients(newCfg),
		)
		slog.Info("Rate limiter configuration updated.")
	})

	slog.Info("Rate limiter initialized and cleanup process started")
	return rl
}

// setupAndRunHealthChecker запускает проверку здоровья бэкендов
func setupAndRunHealthChecker(appCtx context.Context, appWg *sync.WaitGroup, cfg *config.Config, b balancer.Balancer) {
	hc := health.NewChecker(
		cfg.Backends,
		cfg.HealthCheck.IntervalSeconds,
		cfg.HealthCheck.TimeoutSeconds,
		cfg.HealthCheck.Path,
		func(live []string) { b.Update(live) },
	)

	appWg.Add(1)
	go func() {
		defer appWg.Done()
		hc.Start(appCtx)
	}()

	config.Subscribe(func(newCfg *config.Config) {
		slog.Info("Stopping health checker for reconfiguration...")
		hc.Stop() // Блокирующий вызов, дождется остановки
		slog.Info("Health checker stopped. Updating configuration...")
		hc.UpdateConfig(newCfg.Backends,
			newCfg.HealthCheck.IntervalSeconds,
			newCfg.HealthCheck.TimeoutSeconds,
			newCfg.HealthCheck.Path)

		slog.Info("Health checker configuration updated. Restarting...")
		hc.Start(appCtx) // Запускаем новый цикл с обновленной конфигурацией
		slog.Info("Health checker restarted with new configuration.")
	})

	slog.Info("health checker started")
}

func setupHttpServer(cfg *config.Config, handler *server.Handler, l *ratelimiter.Limiter) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/", ratelimiter.Middleware(l, handler))
	mux.HandleFunc("/health", handler.HealthCheck)

	return &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
}

func setupHandler(b balancer.Balancer) *server.Handler {
	return server.NewHandler(b)
}
