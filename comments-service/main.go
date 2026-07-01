package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

// ===== МОДЕЛИ (БЕЗ поля status!) =====

type Comment struct {
	ID        int64     `json:"id"`
	NewsID    int64     `json:"newsId"`
	ParentID  *int64    `json:"parentId,omitempty"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateRequest struct {
	NewsID   int64  `json:"newsId"`
	ParentID *int64 `json:"parentId,omitempty"`
	Text     string `json:"text"`
}

// ===== БАЗА ДАННЫХ (без поля status) =====

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "comments.db")
	if err != nil {
		log.Fatal("db open:", err)
	}
	db.SetMaxOpenConns(1)

	schema := `
	CREATE TABLE IF NOT EXISTS comments (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		news_id    INTEGER NOT NULL,
		parent_id  INTEGER,
		text       TEXT    NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_news_id ON comments(news_id);
	`
	if _, err := db.Exec(schema); err != nil {
		log.Fatal("schema:", err)
	}
	log.Println("✅ Database initialized (comments.db)")
}

// ===== MIDDLEWARE =====

type contextKey string

const reqIDKey contextKey = "request_id"

func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.URL.Query().Get("request_id")
		if reqID == "" {
			reqID = r.Header.Get("X-Request-ID")
		}
		if reqID == "" {
			reqID = generateRequestID()
		}
		ctx := context.WithValue(r.Context(), reqIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		reqID, _ := r.Context().Value(reqIDKey).(string)
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = strings.Split(fwd, ",")[0]
		}

		log.Printf("[%s] %s | %s %s | %d | %v",
			reqID, ip, r.Method, r.URL.Path, wrapped.statusCode, time.Since(start))
	})
}

// ===== ОБРАБОТЧИКИ (простые CRUD, без модерации) =====

func createComment(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.NewsID == 0 || strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "newsId and text required"})
		return
	}

	res, err := db.Exec(
		"INSERT INTO comments (news_id, parent_id, text) VALUES (?, ?, ?)",
		req.NewsID, req.ParentID, req.Text,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	id, _ := res.LastInsertId()
	c := Comment{
		ID:        id,
		NewsID:    req.NewsID,
		ParentID:  req.ParentID,
		Text:      req.Text,
		CreatedAt: time.Now().UTC(),
	}
	writeJSON(w, http.StatusCreated, c)
}

func getComments(w http.ResponseWriter, r *http.Request) {
	newsIDStr := r.URL.Query().Get("news_id")
	newsID, err := strconv.ParseInt(newsIDStr, 10, 64)
	if err != nil || newsID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid news_id"})
		return
	}

	rows, err := db.Query(
		`SELECT id, news_id, parent_id, text, created_at 
		 FROM comments WHERE news_id = ? ORDER BY created_at ASC`,
		newsID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	comments := make([]Comment, 0)
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.NewsID, &c.ParentID, &c.Text, &c.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "rows iteration failed"})
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

// ===== ХЕЛПЕРЫ =====

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ===== ТОЧКА ВХОДА С GRACEFUL SHUTDOWN =====

func main() {
	initDB()
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /comments", createComment)
	mux.HandleFunc("GET /comments", getComments)

	handler := RequestIDMiddleware(LoggingMiddleware(mux))

	server := &http.Server{
		Addr:    ":8081",
		Handler: handler,
	}

	go func() {
		log.Println("[*] Comments Service HTTP server is started on localhost:8081")
		log.Println("[*] Endpoints:")
		log.Println("    POST /comments")
		log.Println("    GET  /comments?news_id={id}")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Printf("[*] Comments Service HTTP server has been stopped. Reason: got %s", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
}
