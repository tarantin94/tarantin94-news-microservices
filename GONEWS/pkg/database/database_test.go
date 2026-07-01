package database

import (
	"testing"
	"time"

	"gonews/pkg/models"
)

// setupTestDB создаёт in-memory SQLite базу и инициализирует схему
func setupTestDB(t *testing.T) *DB {
	cfg := Config{
		Path: ":memory:", // база в оперативной памяти
	}

	db, err := NewDB(cfg)
	if err != nil {
		t.Fatalf("Не удалось создать тестовую БД: %v", err)
	}

	// Создаём таблицу (в in-memory БД её не существует по умолчанию)
	schema := `
	CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		content TEXT,
		pub_time TIMESTAMP,
		link TEXT UNIQUE NOT NULL,
		source TEXT
	);`
	if err := db.InitSchema(schema); err != nil {
		t.Fatalf("Не удалось инициализировать схему: %v", err)
	}

	// Гарантируем закрытие после теста
	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// makePost — хелпер для создания тестовой новости
func makePost(title, link string, pubTime time.Time) models.Post {
	return models.Post{
		Title:   title,
		Content: "Контент для " + title,
		PubTime: pubTime,
		Link:    link,
		Source:  "test",
	}
}

func TestDB_InitSchema(t *testing.T) {
	db := setupTestDB(t)

	// Проверяем, что таблица создана — пробуем сделать SELECT
	_, err := db.GetPosts(1)
	if err != nil {
		t.Errorf("Таблица posts не создана или недоступна: %v", err)
	}
}

func TestDB_SavePost(t *testing.T) {
	db := setupTestDB(t)

	post := makePost("Первая новость", "https://example.com/1", time.Now())
	if err := db.SavePost(post); err != nil {
		t.Fatalf("Не удалось сохранить новость: %v", err)
	}

	posts, err := db.GetPosts(10)
	if err != nil {
		t.Fatalf("Не удалось получить новости: %v", err)
	}
	if len(posts) != 1 {
		t.Errorf("Ожидалась 1 новость, получено %d", len(posts))
	}
	if posts[0].Title != "Первая новость" {
		t.Errorf("Неверный заголовок: %s", posts[0].Title)
	}
}

func TestDB_SavePost_UniqueLink(t *testing.T) {
	db := setupTestDB(t)

	post1 := makePost("Новость 1", "https://example.com/unique", time.Now())
	post2 := makePost("Новость 2", "https://example.com/unique", time.Now()) // тот же link

	if err := db.SavePost(post1); err != nil {
		t.Fatalf("Первое сохранение не удалось: %v", err)
	}

	// Второе сохранение с тем же link должно пройти без ошибки (ON CONFLICT DO NOTHING)
	if err := db.SavePost(post2); err != nil {
		t.Errorf("Повторное сохранение не должно вызывать ошибку: %v", err)
	}

	posts, _ := db.GetPosts(10)
	if len(posts) != 1 {
		t.Errorf("Дубликат link должен был быть проигнорирован. Ожидалась 1 новость, получено %d", len(posts))
	}
}

func TestDB_SavePosts(t *testing.T) {
	db := setupTestDB(t)

	posts := []models.Post{
		makePost("Новость 1", "https://example.com/1", time.Now()),
		makePost("Новость 2", "https://example.com/2", time.Now()),
		makePost("Новость 3", "https://example.com/3", time.Now()),
	}

	saved, err := db.SavePosts(posts)
	if err != nil {
		t.Fatalf("Не удалось сохранить новости: %v", err)
	}
	if saved != 3 {
		t.Errorf("Ожидалось 3 сохранённых, получено %d", saved)
	}
}

func TestDB_GetPosts_OrderAndLimit(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	posts := []models.Post{
		makePost("Старая", "https://example.com/old", now.Add(-3*time.Hour)),
		makePost("Средняя", "https://example.com/mid", now.Add(-2*time.Hour)),
		makePost("Новая", "https://example.com/new", now.Add(-1*time.Hour)),
	}
	db.SavePosts(posts)

	// Запрашиваем только 2, должны получить последние 2 в порядке DESC
	result, err := db.GetPosts(2)
	if err != nil {
		t.Fatalf("Ошибка GetPosts: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Ожидалось 2 новости, получено %d", len(result))
	}
	// Первая должна быть самая свежая
	if result[0].Title != "Новая" {
		t.Errorf("Первая новость должна быть 'Новая', получена '%s'", result[0].Title)
	}
	if result[1].Title != "Средняя" {
		t.Errorf("Вторая новость должна быть 'Средняя', получена '%s'", result[1].Title)
	}
}

func TestDB_GetPostByID(t *testing.T) {
	db := setupTestDB(t)

	post := makePost("Тестовая новость", "https://example.com/test", time.Now())
	db.SavePost(post)

	// Получаем все, чтобы узнать ID
	all, _ := db.GetPosts(1)
	if len(all) == 0 {
		t.Fatal("Новость не сохранилась")
	}

	// Получаем по ID
	found, err := db.GetPostByID(all[0].ID)
	if err != nil {
		t.Fatalf("Не удалось получить новость по ID: %v", err)
	}
	if found.Title != "Тестовая новость" {
		t.Errorf("Неверный заголовок: %s", found.Title)
	}
}

func TestDB_GetPostByID_NotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.GetPostByID(9999)
	if err == nil {
		t.Error("Ожидалась ошибка для несуществующего ID")
	}
}

func TestDB_GetPostsPaginated(t *testing.T) {
	db := setupTestDB(t)

	// Сохраняем 5 новостей
	for i := 1; i <= 5; i++ {
		post := makePost(
			"Новость "+string(rune('0'+i)),
			"https://example.com/"+string(rune('0'+i)),
			time.Now().Add(time.Duration(i)*time.Minute),
		)
		db.SavePost(post)
	}

	// Страница 1, по 2 элемента
	posts, total, err := db.GetPostsPaginated(2, 0)
	if err != nil {
		t.Fatalf("Ошибка GetPostsPaginated: %v", err)
	}
	if total != 5 {
		t.Errorf("Ожидалось всего 5 новостей, получено %d", total)
	}
	if len(posts) != 2 {
		t.Errorf("Ожидалось 2 новости на странице, получено %d", len(posts))
	}

	// Страница 2 (offset 2)
	posts2, total2, err := db.GetPostsPaginated(2, 2)
	if err != nil {
		t.Fatalf("Ошибка GetPostsPaginated (стр. 2): %v", err)
	}
	if total2 != 5 {
		t.Errorf("Total должен быть 5 на любой странице, получено %d", total2)
	}
	if len(posts2) != 2 {
		t.Errorf("Ожидалось 2 новости на 2-й странице, получено %d", len(posts2))
	}

	// Убедимся, что новости на разных страницах разные
	if posts[0].ID == posts2[0].ID {
		t.Error("Новости на разных страницах не должны совпадать")
	}
}

func TestDB_GetPostsPaginatedWithSearch(t *testing.T) {
	db := setupTestDB(t)

	posts := []models.Post{
		makePost("Golang для начинающих", "https://example.com/1", time.Now()),
		makePost("Docker и контейнеры", "https://example.com/2", time.Now()),
		makePost("Продвинутый GOLANG", "https://example.com/3", time.Now()), // с другим регистром
		makePost("SQLite базы данных", "https://example.com/4", time.Now()),
	}
	db.SavePosts(posts)

	// Поиск по "go" (должен найти "Golang для начинающих" и "Продвинутый GOLANG" благодаря LOWER())
	found, total, err := db.GetPostsPaginatedWithSearch("go", 10, 0)
	if err != nil {
		t.Fatalf("Ошибка поиска: %v", err)
	}
	if total != 2 {
		t.Errorf("Ожидалось 2 новости по запросу 'go', получено %d", total)
	}
	if len(found) != 2 {
		t.Errorf("Ожидалось 2 найденных новости, получено %d", len(found))
	}

	// Поиск по несуществующему слову
	found2, total2, err := db.GetPostsPaginatedWithSearch("xyz123", 10, 0)
	if err != nil {
		t.Fatalf("Ошибка пустого поиска: %v", err)
	}
	if total2 != 0 || len(found2) != 0 {
		t.Errorf("Ожидалось 0 новостей, получено %d", len(found2))
	}
}
