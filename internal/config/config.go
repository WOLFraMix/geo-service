package config

import (
	"log" // логирование: предупреждения и фатальные ошибки
	"os"  // чтение переменных окружения
	"time"
)

// Config — все настройки сервиса в одном месте.
// Значения приходят из окружения, дефолты — разумные.
type Config struct {
	HTTPAddr   string        // адрес HTTP-сервера, например ":8080"
	Timeout    time.Duration // таймаут операций (база, исходящие вызовы)
	CHAddr     string        // адрес ClickHouse
	CHPassword string        // пароль пользователя default
	GRPCAddr   string        // адрес gRPC-резолвера (куда ходить API)
	GRPCListen string        // где резолверу слушать: ":50051"
}

// New читает окружение и возвращает Config.
// Fail fast: сломанный конфиг — падаем при старте.
func New() Config {
	cfg := Config{
		HTTPAddr:   envOr("HTTP_ADDR", ":8080"),
		CHAddr:     envOr("CH_ADDR", "clickhouse:9000"),
		CHPassword: envOr("CH_PASSWORD", "dev123"), // дефолт совпадает с compose для локальной разработки
		GRPCAddr:   envOr("GRPC_ADDR", "resolver:50051"),
		GRPCListen: envOr("GRPC_LISTEN", ":50051"),
	}

	// Таймаут парсим через time.ParseDuration: "5s", "100ms" и т.п.
	raw := envOr("TIMEOUT", "5s")
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		log.Fatalf("invalid TIMEOUT %q: %v", raw, err)
	}
	cfg.Timeout = timeout

	return cfg
}

// envOr возвращает значение переменной окружения или дефолт.
// Маленькая хелпер-функция, чтобы не дублировать if-блоки.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
