// Package models содержит общие модели данных
package models

import "time"

// Post представляет публикацию из RSS-ленты
type Post struct {
	ID      int       `json:"id"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
	PubTime time.Time `json:"pub_time"`
	Link    string    `json:"link"`
	Source  string    `json:"source"`
}
