package config

import (
	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"sync"
	"text/template"
	"time"
)

var (
	current     *Config
	mu          sync.RWMutex
	subscribers []func(*Config)
	configPath  string
)

// Load загружает конфигурацию из файла
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Установка значений по умолчанию, если они не указаны
	//loadDefaultValues(&cfg)

	return &cfg,
		nil
}

// Get возвращает копию текущей конфигурации
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Subscribe добавляет функцию-обработчик для обновлений
func Subscribe(callback func(*Config)) {
	mu.Lock()
	defer mu.Unlock()
	subscribers = append(subscribers, callback)
}

// Init инициализирует конфиг и запускает наблюдение
func Init(pathOptional ...string) error {
	templatePath := "configs/config.template.yaml"
	outputPath := os.Getenv("CONFIG_PATH")

	if outputPath == "" && len(pathOptional) > 0 {
		outputPath = pathOptional[0]
	}

	if outputPath == "" {
		outputPath = "configs/config.yaml"
	}
	//configPath = path

	// Загружаем переменные из .env
	_ = godotenv.Load(".env")

	// Подставляем переменные и создаём итоговый YAML
	err := renderConfigFromTemplate(templatePath, outputPath)
	if err != nil {
		return err
	}

	configPath = outputPath
	cfg, err := Load(configPath)
	if err != nil {
		return err
	}

	mu.Lock()
	loadDefaultValues(cfg)
	current = cfg
	mu.Unlock()

	go watch(configPath, templatePath)
	return nil
}

// Watch следит за изменением файла шаблона или .env и обновляет конфиг
func watch(outputPath, templatePath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("failed to create watcher:", err)
		return
	}
	defer watcher.Close()

	if err = watcher.Add(templatePath); err != nil {
		log.Println("failed to watch (template) config file:", err)
		return
	}

	if err = watcher.Add(".env"); err != nil {
		log.Println("warn: failed to watch .env:", err)
		return
	}

	var debounce *time.Timer

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				// debounce, чтобы не перезагружать конфиг многократно при одной операции
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(500*time.Millisecond, func() {
					err := renderConfigFromTemplate(templatePath, outputPath)
					if err != nil {
						log.Printf("failed to render config from template: %v\n", err)
						return
					}

					// Загружаем новый config.yaml
					cfg, err := Load(outputPath)
					if err != nil {
						log.Printf("failed to reload config: %v\n", err)
						return
					}
					mu.Lock()
					current = cfg
					for _, s := range subscribers {
						go s(cfg)
					}
					mu.Unlock()
					log.Println("config reloaded (template change detected)")
				})
			}
		case err := <-watcher.Errors:
			log.Printf("watch error: %v\n", err)
		}
	}
}

// renderConfigFromTemplate обрабатывает шаблон и записывает YAML
func renderConfigFromTemplate(templatePath, outputPath string) error {
	tpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return err
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Получаем переменные окружения
	data := map[string]string{
		"BACKEND_HOST": os.Getenv("BACKEND_HOST"),
		// сюда можно добавить и другие переменные
	}

	return tpl.Execute(outFile, data)
}
