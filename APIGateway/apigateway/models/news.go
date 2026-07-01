package models

import "time"

// NewsShortDetailed — краткая карточка новости (для списков).
type NewsShortDetailed struct {
	ID               int64     `json:"id"`
	Title            string    `json:"title"`
	ShortDescription string    `json:"shortDescription"`
	PublishedAt      time.Time `json:"publishedAt"`
	Author           string    `json:"author"`
	Tags             []string  `json:"tags"`
}

// NewsFullDetailed — полная версия новости с комментариями.
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

// Comment — комментарий к новости.
type Comment struct {
	ID        int64     `json:"id"`
	NewsID    int64     `json:"newsId"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}
