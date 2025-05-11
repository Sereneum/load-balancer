/*
Пакет health реализует:
- Активные проверки здоровья бэкендов
- Механизм callback при изменении состояния
- Горячее обновление параметров проверок
- Параллельное выполнение проверок с таймаутами
*/

package health

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Checker выполняет периодические проверки и обновляет состояние
type Checker struct {
	mu       sync.RWMutex
	backends []string

	interval time.Duration
	timeout  time.Duration
	path     string // Путь для health check, например "/health"

	OnUpdate func([]string) // Callback для уведомления об изменении списка живых серверов

	//healthy map[string]bool

	// Для управления циклом проверок
	activeCtx    context.Context    // Контекст текущего активного цикла проверок
	activeCancel context.CancelFunc // Функция для отмены текущего цикла
	wg           sync.WaitGroup     // Для ожидания завершения горутины проверок
}

func NewChecker(
	initBackends []string,
	initInterval, initTimeout time.Duration,
	initPath string,
	onUpdate func([]string)) *Checker {
	return &Checker{
		backends: append([]string(nil), initBackends...),
		interval: initInterval,
		timeout:  initTimeout,
		path:     initPath,
		OnUpdate: onUpdate,
	}
}

// UpdateConfig останавливает текущий цикл проверок (если он был запущен),
// обновляет конфигурацию и рекомендует перезапустить Start.
func (c *Checker) UpdateConfig(
	newBackends []string,
	newInterval, newTimeout time.Duration,
	newPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	slog.Info("HealthChecker: received config update",
		slog.Any("new_backends", newBackends),
		slog.Duration("new_interval", newInterval),
		slog.Duration("new_timeout", newTimeout),
		slog.String("new_path", newPath))

	// Если цикл был активен, сигнализируем ему об остановке
	if c.activeCancel != nil {
		slog.Debug("HealthChecker: signaling current check cycle to stop due to config update.")
	}

	c.activeCancel() // Сигнал на остановку
	// Не ждем здесь c.wg.Wait(), чтобы не блокировать подписчика конфига надолго.
	// Вызывающий код (в main) должен будет дождаться остановки перед новым Start.
	c.backends = append([]string(nil), newBackends...) // Обновляем с копией
	c.interval = newInterval
	c.timeout = newTimeout
	c.path = newPath
}

// Start запускает цикл проверок здоровья. Если уже запущен, ничего не делает.
// Принимает родительский контекст для общего управления жизненным циклом.
func (c *Checker) Start(parentCtx context.Context) {
	c.mu.Lock() // Блокируем для проверки и установки activeCancel

	if c.activeCancel != nil {
		slog.Info(
			"HealthChecker: Start called, but check cycle is already running or stopping. Ignoring.",
		)
		c.mu.Unlock()
		return
	}

	// Создаем новый контекст для этого цикла проверок, который можно будет отменить
	// и который будет дочерним от parentCtx
	ctx, cancel := context.WithCancel(parentCtx)
	c.activeCtx = ctx
	c.activeCancel = cancel

	// Копируем текущие параметры под мьютексом, чтобы горутина работала с консистентными данными
	currentBackends := append([]string(nil), c.backends...)
	currentInterval := c.interval
	currentTimeout := c.timeout
	currentPath := c.path
	onUpdateCallback := c.OnUpdate

	c.mu.Unlock() // Разблокируем перед запуском горутины

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		slog.Info(
			"HealthChecker: health check loop started",
			slog.Duration("interval", currentInterval),
			slog.String("path", currentPath),
			slog.Any("backends_to_check", currentBackends),
		)

		// Немедленная первая проверка при старте
		c.performChecks(currentBackends, currentTimeout, currentPath, onUpdateCallback)

		ticker := time.NewTicker(currentInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Параметры (backends, timeout, path) были зафиксированы при запуске горутины.
				// Если они изменятся через UpdateConfig, эта горутина будет остановлена
				// и запущена новая с актуальными параметрами.
				c.performChecks(currentBackends, currentTimeout, currentPath, onUpdateCallback)
				//healthy := c.checkAll()
				//if c.OnUpdate != nil {
				//	c.OnUpdate(healthy)
				//}

				// Если c.activeCancel() был вызван или parentCtx отменен
			case <-ctx.Done():
				slog.Info("HealthChecker: health check loop stopping.")
				return
			}
		}
	}()
}

func (c *Checker) Stop() {
	c.mu.Lock()

	if c.activeCancel != nil {
		slog.Info("HealthChecker: Stop called, signaling check loop to terminate.")
		c.activeCancel()
		c.activeCancel = nil // Сбрасываем, чтобы показать, что остановка инициирована
	} else {
		slog.Info("HealthChecker: Stop called, but no active check loop to stop.")
		c.mu.Unlock()
		return
	}

	c.mu.Unlock()
	c.wg.Wait()
	slog.Info("HealthChecker: check loop gracefully stopped.")
}

// performChecks выполняет одну итерацию проверки всех бэкендов
// Принимает параметры как аргументы, чтобы быть уверенным в их консистентности на момент вызова.
func (c *Checker) performChecks(
	backendsToCheck []string,
	checkTimeout time.Duration,
	checkPath string,
	onUpdate func([]string)) {
	if len(backendsToCheck) == 0 {
		slog.Debug("HealthChecker: no backends to check in this round.")
		if onUpdate != nil {
			onUpdate([]string{})
		}
		return
	}

	slog.Debug(
		"HealthChecker: starting a round of health checks",
		slog.Any("backends", backendsToCheck),
	)

	liveBackends := make([]string, 0, len(backendsToCheck))
	var wgChecks sync.WaitGroup
	muLive := &sync.Mutex{}

	// Создаем HTTP клиент для этой сессии проверок
	// DisableKeepAlives: true - не держать лишние соединения к потенциально больным серверам.
	client := http.Client{
		Timeout: checkTimeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	for _, backendAddr := range backendsToCheck {
		wgChecks.Add(1)
		go func(addr string) {
			defer wgChecks.Done()
			urlToCheck := strings.TrimSuffix(addr, "/")
			if !strings.HasPrefix(urlToCheck, "http://") && !strings.HasPrefix(urlToCheck, "https://") {
				urlToCheck = "http://" + urlToCheck
			}

			pathPart := strings.TrimPrefix(checkPath, "/")
			if pathPart != "" {
				urlToCheck = urlToCheck + "/" + pathPart
			}

			req, err := http.NewRequestWithContext(
				c.activeCtx,
				http.MethodGet,
				urlToCheck,
				nil,
			)

			if err != nil {
				slog.Warn(
					"HealthChecker: failed to create request",
					slog.String("backend", addr),
					slog.String("url", urlToCheck),
					slog.String("error", err.Error()))
				return
			}

			resp, err := client.Do(req)
			if err != nil {
				if c.activeCtx.Err() != nil {
					slog.Debug(
						"HealthChecker: request cancelled for backend",
						slog.String("backend", addr),
						slog.String("url", urlToCheck))
				} else {
					slog.Warn(
						"HealthChecker: check failed for backend",
						slog.String("backend", addr),
						slog.String("url", urlToCheck),
						slog.String("error", err.Error()))
				}
				return
			}

			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				muLive.Lock()
				liveBackends = append(liveBackends, addr)
				muLive.Unlock()
			} else {
				slog.Warn(
					"HealthChecker: backend unhealthy",
					slog.String("backend", addr),
					slog.String("url", urlToCheck),
					slog.Int("status_code", resp.StatusCode))
			}
		}(backendAddr)
	}

	wgChecks.Wait()
	sort.Strings(liveBackends)
	slog.Info(
		"HealthChecker: health check round completed",
		slog.Any("live_backends", liveBackends),
		slog.Int("total_checked", len(backendsToCheck)))
	if onUpdate != nil {
		onUpdate(liveBackends)
	}
}

/*
func (c *Checker) checkAll() []string {
	mu := &sync.Mutex{}
	healthy := make([]string, 0, len(c.backends))
	var wg sync.WaitGroup

	client := http.Client{
		Timeout: c.timeout,
		Transport: &http.Transport{
			DisableKeepAlives:   true,
			MaxIdleConnsPerHost: -1,
		},
	}

	for _, backend := range c.backends {
		// Добавляем http:// если нет схемы
		b := backend
		if !strings.HasPrefix(b, "http://") && !strings.HasPrefix(b, "https://") {
			b = "http://" + b
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			req, err := http.NewRequest(http.MethodGet, b+"/health", nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				mu.Lock()
				healthy = append(healthy, backend)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	slog.Info(
		"Health check completed",
		slog.Int("total_backends", len(c.backends)),
		slog.Int("healthy", len(healthy)),
		slog.String("healthy_list", strings.Join(healthy, ", ")),
	)
	return healthy
}
*/
