package main

import (
	"log"
	"net/http"

	"geo-service/internal/config"
)

func main() {
	// 1. Конфигурация из окружения
	cfg := config.New()

	// 2. Роутер. /healthz — стандартный эндпоинт проверки живости:
	//    Docker и будущие health-чеки будут дёргать именно его.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		// http.Error — короткий ответ с текстом и кодом.
		http.Error(w, "ok", http.StatusOK)
	})

	// 3. Сервер с таймаутами — не ListenAndServe, а полный http.Server.
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	}

	log.Printf("api listening on %s", cfg.HTTPAddr)
	log.Fatal(srv.ListenAndServe())
}
