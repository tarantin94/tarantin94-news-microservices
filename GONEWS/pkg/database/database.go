package database

import (
	"database/sql"
	"fmt"

	"gonews/pkg/models"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

// Config для SQLite упрощён — нужен только путь к файлу
type Config struct {
	Path string // например, "./gonews.db"
}

func NewDB(cfg Config) (*DB, error) {
	if cfg.Path == "" {
		cfg.Path = "gonews.db"
	}

	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("ошибка открытия БД: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ошибка проверки подключения: %w", err)
	}

	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) SavePost(post models.Post) error {
	query := `
		INSERT INTO posts (title, content, pub_time, link, source)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(link) DO NOTHING`
	_, err := d.db.Exec(query, post.Title, post.Content, post.PubTime, post.Link, post.Source)
	return err
}

func (d *DB) SavePosts(posts []models.Post) (int, error) {
	saved := 0
	for _, post := range posts {
		if err := d.SavePost(post); err == nil {
			saved++
		}
	}
	return saved, nil
}

func (d *DB) GetPosts(n int) ([]models.Post, error) {
	query := `
		SELECT id, title, content, pub_time, link, source
		FROM posts
		ORDER BY pub_time DESC
		LIMIT ?`

	rows, err := d.db.Query(query, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []models.Post
	for rows.Next() {
		var p models.Post
		err := rows.Scan(&p.ID, &p.Title, &p.Content, &p.PubTime, &p.Link, &p.Source)
		if err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// НОВОЕ: получение новости по ID
func (d *DB) GetPostByID(id int) (models.Post, error) {
	query := `
		SELECT id, title, content, pub_time, link, source
		FROM posts WHERE id = ?`
	var post models.Post
	err := d.db.QueryRow(query, id).Scan(
		&post.ID, &post.Title, &post.Content,
		&post.PubTime, &post.Link, &post.Source,
	)
	return post, err
}

// НОВОЕ: пагинация без поиска
func (d *DB) GetPostsPaginated(perPage, offset int) ([]models.Post, int, error) {
	var total int
	err := d.db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, title, content, pub_time, link, source
		FROM posts ORDER BY pub_time DESC LIMIT ? OFFSET ?`
	rows, err := d.db.Query(query, perPage, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var posts []models.Post
	for rows.Next() {
		var p models.Post
		if err := rows.Scan(&p.ID, &p.Title, &p.Content, &p.PubTime, &p.Link, &p.Source); err != nil {
			return nil, 0, err
		}
		posts = append(posts, p)
	}
	return posts, total, rows.Err()
}

// НОВОЕ: поиск по названию + пагинация (LIKE вместо ILIKE, регистр обрабатываем руками)
func (d *DB) GetPostsPaginatedWithSearch(search string, perPage, offset int) ([]models.Post, int, error) {
	searchPattern := "%" + search + "%"

	var total int
	err := d.db.QueryRow(
		"SELECT COUNT(*) FROM posts WHERE LOWER(title) LIKE LOWER(?)",
		searchPattern,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, title, content, pub_time, link, source
		FROM posts WHERE LOWER(title) LIKE LOWER(?)
		ORDER BY pub_time DESC LIMIT ? OFFSET ?`
	rows, err := d.db.Query(query, searchPattern, perPage, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var posts []models.Post
	for rows.Next() {
		var p models.Post
		if err := rows.Scan(&p.ID, &p.Title, &p.Content, &p.PubTime, &p.Link, &p.Source); err != nil {
			return nil, 0, err
		}
		posts = append(posts, p)
	}
	return posts, total, rows.Err()
}

func (d *DB) InitSchema(schemaSQL string) error {
	_, err := d.db.Exec(schemaSQL)
	return err
}
