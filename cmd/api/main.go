package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"geo-service/internal/config"
	"geo-service/internal/grpcclient"
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

	// 4. Роутер + регистрация ВСЕХ маршрутов в httpapi.New
	res, err := grpcclient.New(cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("connect to resolver: %v", err)
	}
	defer res.Close()

	mux := http.NewServeMux()
	httpapi.New(mux, store, res)

	// 5. Сервер с таймаутами
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	}

	// Канал, в который попадёт сигнал от системы: Ctrl+C (SIGINT)
	// или docker stop / kill (SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Горутина-сервер: ListenAndServe блокируется до ошибки,
	// поэтому выносим её в фон и не блокируем main
	go func() {
		log.Printf("api listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// ErrServerClosed — это НЕ ошибка, а нормальный результат Shutdown;
			// любой другой — фатальная беда
			log.Fatalf("listen: %v", err)
		}
	}()

	// Ждём сигнала — main стоит на этой строке, пока не придёт Ctrl+C
	<-stop
	log.Println("shutdown signal received")

	// Дедлайн вежливости: максимум 10 секунд на доработку текущих запросов,
	// потом всех режем принудительно. Без дедлайна зависший клиент
	// мог бы держать нас вечно
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("http shutdown: %v", err)
	}

	// Закрываем ресурсы: соединение с базой и с резолвером
	store.Close()
	res.Close()

	log.Println("bye")
}
