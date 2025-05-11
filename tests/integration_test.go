package tests

import (
	"flag"
	"fmt"
	"load-balancer/internal/prettylog"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

var configPath string
var mockServer *exec.Cmd
var balancerServer *exec.Cmd

func init() {
	defaultPath := projectRoot()
	flag.StringVar(&configPath, "config", defaultPath, "Path to config file")
}

func startMockServer(cfgPath string) {
	root := projectRoot()
	cmd := exec.Command("go",
		"run",
		filepath.Join(root, "cmd/mockserver/main.go"),
		"-config",
		configPath,
	)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		slog.Error("failed to start mock server", slog.String("error", err.Error()))
		os.Exit(0)
	}
	mockServer = cmd
}

func startBalancerServer(cfgPath string) {
	root := projectRoot()
	cmd := exec.Command(
		"go",
		"run",
		filepath.Join(root, "cmd/balancer/main.go"),
		"-config",
		configPath,
	)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		slog.Error("failed to start balancer server", slog.String("error", err.Error()))
		os.Exit(0)
	}
	balancerServer = cmd
}

func waitForServerReady(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("server %s not ready after %v", url, timeout)
}

func projectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	fmt.Println(filepath.Dir(filename))
	return filepath.Dir(filepath.Dir(filename))
}

func TestMain(m *testing.M) {
	prettylog.InitLogger("debug")
	slog.Info("=== logger initialized from TestMain ===")

	flag.Parse() // обязательно, иначе флаги не будут обработаны
	slog.Info("Using config path", slog.String("configPath", configPath))

	startMockServer(configPath)
	startBalancerServer(configPath)

	err := waitForServerReady("http://localhost:8080/", 10*time.Second)
	if err != nil {
		slog.Error(
			"Failed waiting for server to start",
			slog.String("error", err.Error()),
		)

		if balancerServer != nil && balancerServer.Process != nil {
			balancerServer.Process.Kill()
		}

		if mockServer != nil && mockServer.Process != nil {
			mockServer.Process.Kill()
		}

		os.Exit(1)
	}

	code := m.Run()

	slog.Info("Tests finished. Shutting down external processes...")
	// Отправка SIGINT для корректного завершения
	if balancerServer != nil && balancerServer.Process != nil {
		slog.Info("Sending SIGINT to balancer server...")
		if err := balancerServer.Process.Signal(syscall.SIGINT); err != nil {
			slog.Error("Failed to send SIGINT to balancer server, killing...", slog.String("error", err.Error()))
			balancerServer.Process.Kill()
		}
	}
	if mockServer != nil && mockServer.Process != nil {
		slog.Info("Sending SIGINT to mock server...")
		if err := mockServer.Process.Signal(syscall.SIGINT); err != nil {
			slog.Error("Failed to send SIGINT to mock server, killing...", slog.String("error", err.Error()))
			mockServer.Process.Kill()
		}
	}

	shutdownTimeout := 10 * time.Second // Таймаут на завершение
	serversDone := make(chan struct{}, 2)

	if balancerServer != nil && balancerServer.Process != nil {
		go func() {
			balancerServer.Wait() // Ждем завершения процесса
			slog.Info("Balancer server process exited.")
			serversDone <- struct{}{}
		}()
	} else {
		serversDone <- struct{}{} // Если не был запущен
	}

	if mockServer != nil && mockServer.Process != nil {
		go func() {
			mockServer.Wait() // Ждем завершения процесса
			slog.Info("Mock server process exited.")
			serversDone <- struct{}{}
		}()
	} else {
		serversDone <- struct{}{} // Если не был запущен
	}

	// Ждем завершения обоих или таймаута
	completed := 0
	for completed < 2 {
		select {
		case <-serversDone:
			completed++
		case <-time.After(shutdownTimeout):
			slog.Warn("Timeout waiting for processes to shut down. Forcing kill if any remain.")
			// Принудительно убиваем, если они не завершились по SIGINT
			if balancerServer != nil && balancerServer.ProcessState == nil { // ProcessState is nil if running
				slog.Info("Forcing kill on balancer server.")
				balancerServer.Process.Kill()
			}
			if mockServer != nil && mockServer.ProcessState == nil {
				slog.Info("Forcing kill on mock server.")
				mockServer.Process.Kill()
			}
			completed = 2 // Выходим из цикла
		}
	}

	slog.Info("External processes shut down. Exiting TestMain.")
	os.Exit(code)

}

func TestLoadBalancerIntegration(t *testing.T) {
	url := "http://localhost:8080"
	packs := 20

	users := NewPullUsers(10)
	client := &http.Client{Timeout: 5 * time.Second}

	t.Log("[BEGIN]")
	t.Logf("PACKS: %d\n\n", packs)

	for i := 0; i < packs; i++ {
		t.Logf("BEGIN PACK: %d\n\n", i)
		var wg sync.WaitGroup
		var n = rand.IntN(5) + 30
		wg.Add(n)

		for j := 0; j < n; j++ {
			go func(index int) {
				defer wg.Done()
				// ---
				ip := users.Get()
				req, err := http.NewRequest(http.MethodGet, url, nil)
				if err != nil {
					t.Errorf("[client]-[%s] Ошибка создания запроса: %v", ip, err)
					return
				}
				req.Header.Set("X-Forwarded-For", ip)

				resp, err := client.Do(req)
				if err != nil {
					t.Errorf("[client]-[%s] Запрос %d вернул ошибку: %v", ip, i, err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					switch resp.StatusCode {
					case http.StatusTooManyRequests:
						t.Logf("[client]-[%s] Запрос %d получил: Too Many Requests (429)", ip, i)
					case http.StatusUnauthorized:
						t.Logf("[client]-[%s] Запрос %d получил: Unauthorized (401)", ip, i)
					case http.StatusServiceUnavailable:
						t.Logf("[client]-[%s] Запрос %d получил: No backend available (503)", ip, i)
					case http.StatusBadGateway:
						t.Logf("[client]-[%s] Запрос %d получил: Bad Gateway (502)", ip, i)
					case http.StatusInternalServerError:
						t.Logf("[client]-[%s] Запрос %d получил: Internal Server Error (500)", ip, i)
					default:
						t.Errorf("[client]-[%s] Запрос %d получил неожиданный статус: %d", ip, i, resp.StatusCode)
					}
				}

			}(j)
		}

		wg.Wait()
		t.Logf("END PACK: %d\n\n", i)
		t.Log("---")
		t.Log("---")
		t.Log("---")
		//time.Sleep(time.Duration(rand.Intn(3)+2) * time.Second)
	}

}

type PullUsers struct {
	users []string
}

func NewPullUsers(count int) *PullUsers {
	pu := &PullUsers{}
	pu.users = make([]string, count)
	for i := 0; i < count; i++ {
		pu.users[i] = generateRandomIP()
	}

	return pu
}

func (pu *PullUsers) Get() string {
	return pu.users[rand.IntN(len(pu.users))]
}

func generateRandomIP() string {
	octets := make([]byte, 4)
	for i := range octets {
		switch i {
		case 0:
			octets[i] = byte(rand.IntN(223) + 1)
		case 1:
			octets[i] = byte(rand.IntN(254) + 1)
		default:
			octets[i] = byte(rand.IntN(255))
		}
	}

	return net.IPv4(octets[0], octets[1], octets[2], octets[3]).String()
}
