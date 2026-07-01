package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Comment struct {
	ID        int64     `json:"id"`
	NewsID    int64     `json:"newsId"`
	ParentID  *int64    `json:"parentId,omitempty"`
	Text      string    `json:"text"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateRequest struct {
	NewsID   int64  `json:"newsId"`
	ParentID *int64 `json:"parentId,omitempty"`
	Text     string `json:"text"`
}

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
		status     TEXT    NOT NULL DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_news_id ON comments(news_id);
	`
	if _, err := db.Exec(schema); err != nil {
		log.Fatal("schema:", err)
	}
	log.Println(" Database initialized (comments.db)")
}

var (
	moderationQueue = make(chan int64, 100)
	blacklist       = []string{"qwerty", "йцукен", "zxvbnm"}
)

func startModerator() {
	go func() {
		for id := range moderationQueue {
			moderate(id)
		}
	}()
	log.Println(" Moderation worker started")
}

func moderate(id int64) {
	var text, status string
	err := db.QueryRow("SELECT text, status FROM comments WHERE id = ?", id).
		Scan(&text, &status)
	if err != nil {
		log.Printf("moderate fetch error (id=%d): %v", id, err)
		return
	}
	if status != "pending" {
		return
	}

	lower := strings.ToLower(text)
	blocked := false
	for _, w := range blacklist {
		if strings.Contains(lower, strings.ToLower(w)) {
			blocked = true
			break
		}
	}

	newStatus := "approved"
	if blocked {
		newStatus = "blocked"
	}

	if _, err := db.Exec("UPDATE comments SET status = ? WHERE id = ?", newStatus, id); err != nil {
		log.Printf("moderate update error (id=%d): %v", id, err)
		return
	}
	log.Printf("🔍 Comment %d → %s", id, newStatus)
}

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
		"INSERT INTO comments (news_id, parent_id, text, status) VALUES (?, ?, ?, 'pending')",
		req.NewsID, req.ParentID, req.Text,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	id, _ := res.LastInsertId()

	select {
	case moderationQueue <- id:
	default:
		log.Printf(" queue full for id=%d", id)
	}

	c := Comment{
		ID:        id,
		NewsID:    req.NewsID,
		ParentID:  req.ParentID,
		Text:      req.Text,
		Status:    "pending",
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
		`SELECT id, news_id, parent_id, text, status, created_at 
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
		if err := rows.Scan(&c.ID, &c.NewsID, &c.ParentID, &c.Text, &c.Status, &c.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		comments = append(comments, c)
	}

	// ПРОВЕРКА ОШИБКИ ИТЕРАЦИИ — именно этого требовал линтер
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "rows iteration failed"})
		return
	}

	writeJSON(w, http.StatusOK, comments)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func main() {
	initDB()
	defer db.Close()

	startModerator()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /comments", createComment)
	mux.HandleFunc("GET /comments", getComments)

	addr := ":8081"
	log.Printf(" Comments Service on http://localhost%s/", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
