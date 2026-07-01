// Package rss предоставляет функции для работы с RSS-лентами
package rss

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gonews/pkg/models"

	"github.com/mmcdole/gofeed"
)

// RSSResult содержит результат парсинга одной RSS-ленты
type RSSResult struct {
	Posts []models.Post
	Error error
	URL   string
}

// FeedParser определяет интерфейс для парсинга RSS
type FeedParser interface {
	ParseFeed(feedURL string) ([]models.Post, error)
}

// GoFeedParser реализация парсера на основе библиотеки gofeed
type GoFeedParser struct{}

// NewGoFeedParser создает новый экземпляр парсера RSS
func NewGoFeedParser() *GoFeedParser {
	return &GoFeedParser{}
}

// ParseFeed парсит RSS-ленту по указанному URL и возвращает массив публикаций
func (p *GoFeedParser) ParseFeed(feedURL string) ([]models.Post, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(feedURL)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга ленты %s: %w", feedURL, err)
	}

	posts := make([]models.Post, 0, len(feed.Items))
	for _, item := range feed.Items {
		pubTime := time.Now()
		if item.PublishedParsed != nil {
			pubTime = *item.PublishedParsed
		}

		post := models.Post{
			Title:   item.Title,
			Content: item.Description,
			PubTime: pubTime,
			Link:    item.Link,
			Source:  feed.Title,
		}
		posts = append(posts, post)
	}

	return posts, nil
}

// FetchAllFeeds параллельно загружает все RSS-ленты в отдельных горутинах
// Возвращает канал с результатами парсинга
func FetchAllFeeds(ctx context.Context, parser FeedParser, urls []string) <-chan RSSResult {
	results := make(chan RSSResult, len(urls))
	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go func(feedURL string) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				results <- RSSResult{URL: feedURL, Error: ctx.Err()}
				return
			default:
				posts, err := parser.ParseFeed(feedURL)
				results <- RSSResult{
					Posts: posts,
					Error: err,
					URL:   feedURL,
				}
			}
		}(url)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}
