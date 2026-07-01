package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// ===== ДАННЫЕ =====

var (
	shortNews = []NewsShortDetailed{
		{
			ID: 1, Title: "Запуск API Gateway",
			ShortDescription: "Мы запустили новый шлюз.",
			PublishedAt:      time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
			Author:           "Команда платформы",
			Tags:             []string{"release", "infra"},
		},
		{
			ID: 2, Title: "Обновление документации",
			ShortDescription: "Описали новые эндпоинты.",
			PublishedAt:      time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC),
			Author:           "DevRel",
			Tags:             []string{"docs"},
		},
	}

	fullNews = NewsFullDetailed{
		ID:              1,
		Title:           "Запуск API Gateway",
		FullDescription: "Подробный рассказ о том, как мы проектировали шлюз...",
		PublishedAt:     time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Author:          "Команда платформы",
		Tags:            []string{"release", "infra"},
		ImageURL:        "https://cdn.example.com/news/1/cover.png",
		Comments: []Comment{
			{
				ID:        101,
				NewsID:    1,
				Author:    "Анна",
				Text:      "Отличная работа!",
				CreatedAt: time.Date(2026, 6, 24, 11, 0, 0, 0, time.UTC),
			},
		},
	}
)

// ===== ОБРАБОТЧИКИ =====

func ListNews(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, shortNews)
}

func FilterNews(w http.ResponseWriter, r *http.Request) {
	tag := r.URL.Query().Get("tag")
	author := r.URL.Query().Get("author")

	var result []NewsShortDetailed
	for _, n := range shortNews {
		if tag != "" && !contains(n.Tags, tag) {
			continue
		}
		if author != "" && n.Author != author {
			continue
		}
		result = append(result, n)
	}
	writeJSON(w, http.StatusOK, result)
}

func GetNewsDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id != 1 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, fullNews)
}

func AddComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Author string `json:"author"`
		Text   string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	created := Comment{
		ID:        999,
		NewsID:    1,
		Author:    in.Author,
		Text:      in.Text,
		CreatedAt: time.Now().UTC(),
	}
	writeJSON(w, http.StatusCreated, created)
}

// ===== ХЕЛПЕРЫ =====

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ===== ТОЧКА ВХОДА =====

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /news", ListNews)
	mux.HandleFunc("GET /news/filter", FilterNews)
	mux.HandleFunc("GET /news/{id}", GetNewsDetail)
	mux.HandleFunc("POST /news/{id}/comments", AddComment)

	log.Println("API Gateway is listening on http://localhost:8080/")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
