package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gonews/pkg/api"
	"gonews/pkg/database"
	"gonews/pkg/models"
	"gonews/pkg/rss"
)

// Config конфигурация всего приложения
type Config struct {
	Database      database.Config `json:"database"`
	RSS           []string        `json:"rss"`
	RequestPeriod int             `json:"request_period"`
	Server        ServerConfig    `json:"server"`
}

// ServerConfig конфигурация HTTP-сервера
type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func main() {
	// Загрузка конфигурации из файла
	cfg, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	// Подключение к базе данных
	db, err := database.NewDB(cfg.Database)
	if err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}
	defer db.Close()

	// Инициализация схемы БД
	schema, err := os.ReadFile("../../schema.sql")
	if err != nil {
		log.Fatalf("Ошибка чтения схемы БД: %v", err)
	}

	if err := db.InitSchema(string(schema)); err != nil {
		log.Fatalf("Ошибка инициализации схемы БД: %v", err)
	}

	// Создание RSS парсера
	parser := rss.NewGoFeedParser()

	// Каналы для обработки результатов и ошибок
	postsChan := make(chan models.Post, 100)
	errorChan := make(chan error, 10)

	// Контекст для управляемой остановки
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Воркер для сохранения публикаций в БД
	go func() {
		for post := range postsChan {
			if err := db.SavePost(post); err != nil {
				log.Printf("Ошибка сохранения поста: %v", err)
			}
		}
	}()

	// Воркер для логирования ошибок
	go func() {
		for err := range errorChan {
			log.Printf("Ошибка: %v", err)
		}
	}()

	// Функция обхода всех RSS-лент
	fetchRSS := func() {
		log.Println("Начало обхода RSS-лент...")

		results := rss.FetchAllFeeds(ctx, parser, cfg.RSS)

		for result := range results {
			if result.Error != nil {
				errorChan <- fmt.Errorf("ошибка ленты %s: %w", result.URL, result.Error)
				continue
			}

			for _, post := range result.Posts {
				postsChan <- post
			}

			log.Printf("Обработана лента %s, получено %d публикаций", result.URL, len(result.Posts))
		}

		log.Println("Обход RSS-лент завершен")
	}

	// Первый запуск парсинга
	fetchRSS()

	// Периодический запуск по таймеру
	ticker := time.NewTicker(time.Duration(cfg.RequestPeriod) * time.Minute)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				fetchRSS()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Создание и настройка API сервера
	apiServer := api.NewAPI(db)

	// Запуск HTTP сервера
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Сервер запущен на %s", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: apiServer,
	}

	// Обработка сигналов завершения (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Получен сигнал остановки...")
		cancel()

		// Graceful shutdown с таймаутом
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Ошибка остановки сервера: %v", err)
		}

		close(postsChan)
		close(errorChan)
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Ошибка сервера: %v", err)
	}

	log.Println("Сервер остановлен")
}

// loadConfig загружает и парсит файл конфигурации JSON
func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
