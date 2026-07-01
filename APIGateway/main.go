package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ===== МОДЕЛИ =====

type NewsShortDetailed struct {
	ID               int64     `json:"id"`
	Title            string    `json:"title"`
	ShortDescription string    `json:"shortDescription"`
	PublishedAt      time.Time `json:"publishedAt"`
	Author           string    `json:"author"`
	Tags             []string  `json:"tags"`
}

type NewsFullDetailed struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	FullDescription string    `json:"fullDescription"`
	PublishedAt     time.Time `json:"publishedAt"`
	Author          string    `json:"author"`
	Tags            []string  `json:"tags"`
	ImageURL        string    `json:"imageUrl"`
	Comments        []Comment `json:"comments"`
}

type Comment struct {
	ID        int64     `json:"id"`
	NewsID    int64     `json:"newsId"`
	ParentID  *int64    `json:"parentId,omitempty"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// ===== КОНФИГУРАЦИЯ =====

const (
	newsServiceURL     = "http://localhost:8082"
	commentsServiceURL = "http://localhost:8081"
)

// ===== ОБРАБОТЧИКИ =====

// GET /news — пробрасываем параметры s и page в GONEWS
func GetNewsList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqID, _ := ctx.Value(reqIDKey).(string)

	url := fmt.Sprintf("%s/news/100?request_id=%s", newsServiceURL, reqID)
	if s := r.URL.Query().Get("s"); s != "" {
		url += "&s=" + s
	}
	if page := r.URL.Query().Get("page"); page != "" {
		url += "&page=" + page
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("X-Request-ID", reqID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "news service unavailable"})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// GET /news/{id} — асинхронная агрегация новости + комментариев
func GetNewsDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	newsID := r.PathValue("id")
	reqID, _ := ctx.Value(reqIDKey).(string)

	results := make(chan interface{}, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	// Горутина 1: детальная новость из GONEWS
	go func() {
		defer wg.Done()
		url := fmt.Sprintf("%s/news/detail/%s?request_id=%s", newsServiceURL, newsID, reqID)
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("X-Request-ID", reqID)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			results <- err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			results <- fmt.Errorf("news service returned %d", resp.StatusCode)
			return
		}

		var news NewsFullDetailed
		if err := json.NewDecoder(resp.Body).Decode(&news); err != nil {
			results <- err
			return
		}
		results <- news
	}()

	// Горутина 2: комментарии из Comments Service
	go func() {
		defer wg.Done()
		url := fmt.Sprintf("%s/comments?news_id=%s&request_id=%s", commentsServiceURL, newsID, reqID)
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("X-Request-ID", reqID)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			results <- err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			results <- fmt.Errorf("comments service returned %d", resp.StatusCode)
			return
		}

		var comments []Comment
		if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
			results <- err
			return
		}
		results <- comments
	}()

	wg.Wait()
	close(results)

	var finalNews NewsFullDetailed
	var finalComments []Comment
	var fetchErr error

	for res := range results {
		switch v := res.(type) {
		case error:
			fetchErr = v
		case NewsFullDetailed:
			finalNews = v
		case []Comment:
			finalComments = v
		}
	}

	if fetchErr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fetchErr.Error()})
		return
	}

	finalNews.Comments = finalComments
	writeJSON(w, http.StatusOK, finalNews)
}

// POST /news/{id}/comments — создание комментария (С ЦЕНЗУРОЙ в API Gateway)
func CreateComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqID, _ := ctx.Value(reqIDKey).(string)

	var req struct {
		NewsID   int64  `json:"newsId"`
		ParentID *int64 `json:"parentId,omitempty"`
		Text     string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	// 🔴 ЦЕНЗУРА ТЕПЕРЬ В API GATEWAY
	if !isAllowed(req.Text) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "comment contains forbidden words",
		})
		return
	}

	url := fmt.Sprintf("%s/comments?request_id=%s", commentsServiceURL, reqID)
	body, _ := json.Marshal(req)
	proxyReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("X-Request-ID", reqID)

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "comments service unavailable"})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// ===== ХЕЛПЕРЫ =====

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ===== ТОЧКА ВХОДА С GRACEFUL SHUTDOWN =====

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /news", GetNewsList)
	mux.HandleFunc("GET /news/{id}", GetNewsDetail)
	mux.HandleFunc("POST /news/{id}/comments", CreateComment)

	handler := RequestIDMiddleware(LoggingMiddleware(mux))

	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	go func() {
		log.Println("[*] API Gateway HTTP server is started on localhost:8080")
		log.Println("[*] Endpoints:")
		log.Println("    GET  /news[?s=...][&page=...]")
		log.Println("    GET  /news/{id}")
		log.Println("    POST /news/{id}/comments")
		log.Printf("[*] Downstream services:")
		log.Printf("    → News Service: %s", newsServiceURL)
		log.Printf("    → Comments Service: %s", commentsServiceURL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Printf("[*] API Gateway HTTP server has been stopped. Reason: got %s", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
}
