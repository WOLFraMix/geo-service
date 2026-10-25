package main

import (
	"context"
	"log"
	"net/http"

	"geo-service/internal/config"
	"geo-service/internal/httpapi"
	"geo-service/internal/storage"
)

func main() {
	// 1. Конфигурация
	cfg := config.New()

	// 2. Контекст с таймаутом для старта
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// 3. Соединение с базой
	store, err := storage.New(ctx, cfg.CHAddr, cfg.CHPassword)
	if err != nil {
		log.Fatalf("connect to clickhouse: %v", err)
	}
	if err := store.Init(ctx); err != nil {
		log.Fatalf("init schema: %v", err)
	}
	log.Printf("clickhouse connected: %s", cfg.CHAddr)

	// 4. Роутер + регистрация ВСЕХ маршрутов в httpapi.New.
	//    Временный smoke-test из фазы 2 удалён — его роль теперь
	//    играет полноценный listCities с лимитом и валидацией.
	mux := http.NewServeMux()
	httpapi.New(mux, store)

	// 5. Сервер с таймаутами
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	}

	log.Printf("api listening on %s", cfg.HTTPAddr)
	log.Fatal(srv.ListenAndServe())
}
