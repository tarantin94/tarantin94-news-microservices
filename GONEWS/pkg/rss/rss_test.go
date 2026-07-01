package rss

import (
	"context"
	"testing"
	"time"

	"gonews/pkg/models"
)

func TestGoFeedParser_ParseFeed(t *testing.T) {
	parser := NewGoFeedParser()

	posts, err := parser.ParseFeed("https://habr.com/ru/rss/hub/go/all/?fl=ru")
	if err != nil {
		t.Skipf("Пропускаем тест: невозможно подключиться к RSS ленте: %v", err)
	}

	if len(posts) == 0 {
		t.Error("Ожидались публикации, но получено 0")
	}

	for _, post := range posts {
		if post.Title == "" {
			t.Error("Заголовок не должен быть пустым")
		}
		if post.Link == "" {
			t.Error("Ссылка не должна быть пустой")
		}
	}
}

func TestFetchAllFeeds(t *testing.T) {
	parser := NewGoFeedParser()
	urls := []string{
		"https://habr.com/ru/rss/hub/go/all/?fl=ru",
	}

	ctx := context.Background()
	results := FetchAllFeeds(ctx, parser, urls)

	resultCount := 0
	for result := range results {
		resultCount++
		if result.Error != nil {
			t.Logf("Ошибка при парсинге %s: %v", result.URL, result.Error)
			continue
		}
		if len(result.Posts) == 0 {
			t.Errorf("Ожидались публикации из %s", result.URL)
		}
	}

	if resultCount != len(urls) {
		t.Errorf("Ожидалось %d результатов, получено %d", len(urls), resultCount)
	}
}

func TestPostModel(t *testing.T) {
	post := models.Post{
		Title:   "Test Title",
		Content: "Test Content",
		PubTime: time.Now(),
		Link:    "https://example.com",
		Source:  "Test Source",
	}

	if post.Title != "Test Title" {
		t.Error("Неверный заголовок")
	}
}
