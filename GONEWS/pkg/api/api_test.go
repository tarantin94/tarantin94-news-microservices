package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gonews/pkg/database"
	"gonews/pkg/models"
)

// setupTestAPI создает тестовый API с in-memory базой данных SQLite
// и инициализирует схему + тестовые данные
func setupTestAPI(t *testing.T) *API {
	// 1. Создаём in-memory базу (живёт только в рамках теста)
	cfg := database.Config{
		Path: ":memory:",
	}

	db, err := database.NewDB(cfg)
	if err != nil {
		t.Fatalf("Не удалось создать тестовую БД: %v", err)
	}

	// 2. Создаём таблицу posts (в in-memory БД она не существует по умолчанию)
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

	// 3. Добавляем тестовые данные
	testPosts := []models.Post{
		{
			Title:   "Первая новость про Golang",
			Content: "Описание первой новости",
			PubTime: time.Now().Add(-2 * time.Hour),
			Link:    "https://example.com/1",
			Source:  "test",
		},
		{
			Title:   "Вторая новость про Docker",
			Content: "Описание второй новости",
			PubTime: time.Now().Add(-1 * time.Hour),
			Link:    "https://example.com/2",
			Source:  "test",
		},
		{
			Title:   "Третья новость про SQLite",
			Content: "Описание третьей новости",
			PubTime: time.Now(),
			Link:    "https://example.com/3",
			Source:  "test",
		},
	}
	for _, p := range testPosts {
		if err := db.SavePost(p); err != nil {
			t.Fatalf("Не удалось сохранить тестовую новость: %v", err)
		}
	}

	// 4. Гарантируем закрытие БД после завершения теста
	t.Cleanup(func() {
		db.Close()
	})

	return NewAPI(db)
}

// TestAPI_GetNews проверяет получение списка новостей
func TestAPI_GetNews(t *testing.T) {
	api := setupTestAPI(t)

	req := httptest.NewRequest("GET", "/news/10", nil)
	w := httptest.NewRecorder()

	api.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Ожидался Content-Type application/json, получен %s", contentType)
	}

	// Проверяем структуру ответа (с пагинацией)
	var response struct {
		News       []models.Post `json:"news"`
		Pagination struct {
			TotalPages  int `json:"total_pages"`
			CurrentPage int `json:"current_page"`
			PerPage     int `json:"per_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Не удалось распарсить ответ: %v", err)
	}

	if len(response.News) != 3 {
		t.Errorf("Ожидалось 3 новости, получено %d", len(response.News))
	}
	if response.Pagination.CurrentPage != 1 {
		t.Errorf("Ожидалась страница 1, получена %d", response.Pagination.CurrentPage)
	}
}

// TestAPI_GetNews_InvalidNumber проверяет обработку невалидного параметра n
func TestAPI_GetNews_InvalidNumber(t *testing.T) {
	api := setupTestAPI(t)

	req := httptest.NewRequest("GET", "/news/abc", nil)
	w := httptest.NewRecorder()

	api.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Ожидался статус 400, получен %d", w.Code)
	}
}

// TestAPI_GetNews_WithSearch проверяет поиск по названию
func TestAPI_GetNews_WithSearch(t *testing.T) {
	api := setupTestAPI(t)

	req := httptest.NewRequest("GET", "/news/10?s=golang", nil)
	w := httptest.NewRecorder()

	api.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", w.Code)
	}

	var response struct {
		News []models.Post `json:"news"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Не удалось распарсить ответ: %v", err)
	}

	// Должна вернуться только 1 новость с "Golang" в заголовке
	if len(response.News) != 1 {
		t.Errorf("Ожидалась 1 новость по запросу 'golang', получено %d", len(response.News))
	}
	if len(response.News) > 0 && response.News[0].Title != "Первая новость про Golang" {
		t.Errorf("Неверная новость: %s", response.News[0].Title)
	}
}

// TestAPI_GetNewsDetail проверяет получение одной новости
func TestAPI_GetNewsDetail(t *testing.T) {
	api := setupTestAPI(t)

	req := httptest.NewRequest("GET", "/news/detail/1", nil)
	w := httptest.NewRecorder()

	api.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", w.Code)
	}

	var post models.Post
	if err := json.Unmarshal(w.Body.Bytes(), &post); err != nil {
		t.Fatalf("Не удалось распарсить ответ: %v", err)
	}

	if post.ID != 1 {
		t.Errorf("Ожидался ID 1, получен %d", post.ID)
	}
}

// TestAPI_GetNewsDetail_NotFound проверяет 404 для несуществующей новости
func TestAPI_GetNewsDetail_NotFound(t *testing.T) {
	api := setupTestAPI(t)

	req := httptest.NewRequest("GET", "/news/detail/9999", nil)
	w := httptest.NewRecorder()

	api.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Ожидался статус 404, получен %d", w.Code)
	}
}

// TestAPI_Router проверяет, что маршрутизатор создается
func TestAPI_Router(t *testing.T) {
	api := setupTestAPI(t)
	router := api.GetRouter()

	if router == nil {
		t.Error("Маршрутизатор не должен быть nil")
	}
}

// TestAPI_RequestIDMiddleware проверяет генерацию request_id
func TestAPI_RequestIDMiddleware(t *testing.T) {
	api := setupTestAPI(t)

	// 1. Запрос БЕЗ request_id — должен быть сгенерирован
	req := httptest.NewRequest("GET", "/news/10", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", w.Code)
	}

	// 2. Запрос С request_id — должен быть передан дальше
	req = httptest.NewRequest("GET", "/news/10?request_id=my-custom-id", nil)
	w = httptest.NewRecorder()
	api.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", w.Code)
	}
}
