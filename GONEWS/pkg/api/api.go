// Package api предоставляет HTTP API для агрегатора новостей
package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"gonews/pkg/database"
	"gonews/pkg/models"

	"github.com/gorilla/mux"
)

// API представляет HTTP API сервер с маршрутизацией
type API struct {
	router *mux.Router
	db     *database.DB
}

// NewAPI создает новый экземпляр API сервера с настроенными маршрутами
func NewAPI(db *database.DB) *API {
	api := &API{
		router: mux.NewRouter(),
		db:     db,
	}
	api.setupRoutes()
	return api
}

// setupRoutes настраивает маршруты для API и раздачи статики
func (a *API) setupRoutes() {
	// Подключаем middleware (порядок важен!)
	a.router.Use(RequestIDMiddleware)
	a.router.Use(LoggingMiddleware)

	// Эндпоинт для получения новостей: /news/{n}
	a.router.HandleFunc("/news/{n}", a.getNews).Methods("GET", "OPTIONS")

	// НОВЫЙ эндпоинт для детальной новости: /news/detail/{id}
	a.router.HandleFunc("/news/detail/{id}", a.getNewsDetail).Methods("GET")

	// Раздача статических файлов веб-интерфейса
	a.router.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("./webapp"))))
}

// getNews обработчик HTTP-запроса для получения последних новостей
func (a *API) getNews(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	nStr := vars["n"]

	n, err := strconv.Atoi(nStr)
	if err != nil || n <= 0 {
		http.Error(w, "Неверное количество новостей", http.StatusBadRequest)
		return
	}

	// Ограничиваем максимальное количество новостей
	if n > 100 {
		n = 100
	}

	// НОВОЕ: параметры поиска и пагинации
	search := r.URL.Query().Get("s")
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}

	const perPage = 15
	offset := (page - 1) * perPage

	var posts []models.Post
	var totalItems int

	if search != "" {
		posts, totalItems, err = a.db.GetPostsPaginatedWithSearch(search, perPage, offset)
	} else {
		posts, totalItems, err = a.db.GetPostsPaginated(perPage, offset)
	}

	if err != nil {
		http.Error(w, "Ошибка получения новостей", http.StatusInternalServerError)
		return
	}

	totalPages := int(math.Ceil(float64(totalItems) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}

	// Формируем ответ с пагинацией
	response := map[string]interface{}{
		"news": posts,
		"pagination": map[string]int{
			"total_pages":  totalPages,
			"current_page": page,
			"per_page":     perPage,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(response)
}

// getNewsDetail обработчик для получения одной конкретной новости
func (a *API) getNewsDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "Неверный ID новости", http.StatusBadRequest)
		return
	}

	post, err := a.db.GetPostByID(id)
	if err != nil {
		http.Error(w, "Новость не найдена", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(post)
}

// GetRouter возвращает настроенный маршрутизатор для использования в http.Server
func (a *API) GetRouter() *mux.Router {
	return a.router
}

// ServeHTTP реализует интерфейс http.Handler
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}
