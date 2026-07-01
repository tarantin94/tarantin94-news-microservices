package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
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

	// Инициализация схемы БД — пробуем несколько путей
	schema, schemaPath, err := loadSchema()
	if err != nil {
		log.Fatalf("Ошибка загрузки схемы БД: %v", err)
	}
	log.Printf("Схема БД загружена из: %s", schemaPath)

	if err := db.InitSchema(schema); err != nil {
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

	// WaitGroup для ожидания завершения воркеров
	var wg sync.WaitGroup

	// Воркер для сохранения публикаций в БД
	wg.Add(1)
	go func() {
		defer wg.Done()
		for post := range postsChan {
			if err := db.SavePost(post); err != nil {
				log.Printf("Ошибка сохранения поста: %v", err)
			}
		}
	}()

	// Воркер для логирования ошибок
	wg.Add(1)
	go func() {
		defer wg.Done()
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

	// Формирование адреса
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	server := &http.Server{
		Addr:    addr,
		Handler: apiServer,
	}

	// Запуск HTTP сервера в отдельной горутине
	go func() {
		log.Printf("[*] GONEWS HTTP server is started on %s", addr)
		log.Println("[*] Endpoints:")
		log.Println("    GET /news/{n}[?s=...][&page=...]")
		log.Println("    GET /news/detail/{id}")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка сервера: %v", err)
		}
	}()

	// Обработка сигналов завершения (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	log.Printf("[*] GONEWS HTTP server has been stopped. Reason: got %s", sig)

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Ошибка остановки HTTP-сервера: %v", err)
	}

	cancel()
	close(postsChan)
	close(errorChan)
	wg.Wait()

	if err := db.Close(); err != nil {
		log.Printf("Ошибка закрытия БД: %v", err)
	}

	log.Println("[*] GONEWS has been gracefully shut down")
}

// loadSchema пробует несколько путей для загрузки schema.sql
// Это решает проблему зависимости от рабочей директории
func loadSchema() (string, string, error) {
	possiblePaths := []string{
		"../../../schema.sql",     // запуск из cmd/gonews/
		"./schema.sql",            // запуск из GONEWS/
		"./cmd/gonews/schema.sql", // запуск из GONEWS/ (файл рядом с main.go)
		"../../schema.sql",        // запуск из GONEWS/ (если файл в корне monorepo)
	}

	var lastErr error
	for _, path := range possiblePaths {
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data), path, nil
		}
		lastErr = err
	}

	return "", "", fmt.Errorf("не удалось найти schema.sql ни по одному из путей. Последняя ошибка: %v", lastErr)
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
