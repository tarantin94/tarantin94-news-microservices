package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"apigateway/models"
)

// Hard-coded данные. В боевом проекте они придут из News-сервиса.
var (
	shortNews = []models.NewsShortDetailed{
		{
			ID: 1, Title: "Запуск API Gateway",
			ShortDescription: "Мы запустили новый шлюз.",
			PublishedAt:      time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
			Author:           "Команда платформы", Tags: []string{"release", "infra"},
		},
		{
			ID: 2, Title: "Обновление документации",
			ShortDescription: "Описали новые эндпоинты.",
			PublishedAt:      time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC),
			Author:           "DevRel", Tags: []string{"docs"},
		},
	}

	fullNews = models.NewsFullDetailed{
		ID: 1, Title: "Запуск API Gateway",
		FullDescription: "Подробный рассказ о том, как мы проектировали шлюз...",
		PublishedAt:     time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
		Author:          "Команда платформы", Tags: []string{"release", "infra"},
		ImageURL: "https://cdn.example.com/news/1/cover.png",
		Comments: []models.Comment{
			{ID: 101, NewsID: 1, Author: "Анна", Text: "Отличная работа!",
				CreatedAt: time.Date(2026, 6, 24, 11, 0, 0, 0, time.UTC)},
		},
	}
)

// ListNews — GET /news
func ListNews(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, shortNews)
}

// FilterNews — GET /news/filter?tag=...&author=...
// Пока возвращаем тот же список, но фильтр уже предусмотрен.
func FilterNews(w http.ResponseWriter, r *http.Request) {
	tag := r.URL.Query().Get("tag")
	author := r.URL.Query().Get("author")

	var result []models.NewsShortDetailed
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

// GetNewsDetail — GET /news/{id}
func GetNewsDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id") // Go 1.22+; для chi — chi.URLParam
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id != 1 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, fullNews)
}

// AddComment — POST /news/{id}/comments
// Тело: { "author": "...", "text": "..." }
func AddComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Author string `json:"author"`
		Text   string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	created := models.Comment{
		ID:        999,
		NewsID:    1,
		Author:    in.Author,
		Text:      in.Text,
		CreatedAt: time.Now().UTC(),
	}
	writeJSON(w, http.StatusCreated, created)
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
