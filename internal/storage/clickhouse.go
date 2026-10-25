package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2" // официальный драйвер ClickHouse

	"geo-service/internal/geo" // наша модель City
)

// Store — обёртка над соединением с ClickHouse.
// Все запросы к базе живут здесь; бизнес-логика (пакет geo)
// про протокол и SQL не знает вообще — это разделение окупится,
// когда будем добавлять новые запросы.
type Store struct {
	conn clickhouse.Conn // интерфейс соединения из драйвера
}

// New открывает соединение и проверяет его живость.
// addr — это host:port NATIVE-протокола, например "localhost:9000".
func New(ctx context.Context, addr string, password string) (*Store, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr}, // драйвер принимает список — так делают кластеры
		Auth: clickhouse.Auth{
			Database: "default", // база по умолчанию
			Username: "default",
			Password: password, // теперь совпадает с CLICKHOUSE_PASSWORD из compose
		},
		DialTimeout: 5 * time.Second, // сколько ждать установления TCP
		// Сжатие трафика LZ4 — дефолт ClickHouse, дешёвое по CPU:
		// на реальных объёмах заметно экономит сеть
		Compression: &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	})
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}

	// Open не подключается мгновенно — пинг подтверждает,
	// что база реально доступна. Падаем сразу, если нет (fail fast).
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping clickhouse: %w", err)
	}

	return &Store{conn: conn}, nil
}

// Init создаёт таблицу, если её ещё нет. Idempotent: можно звать при каждом старте.
func (s *Store) Init(ctx context.Context) error {
	const schema = `
		CREATE TABLE IF NOT EXISTS cities
		(
			id         UInt64    MATERIALIZED cityHash64(name, lat, lon),
			name       String,
			region     String,
			country    String,
			lat        Float64,
			lon        Float64,
			deleted    UInt8     DEFAULT 0,
			created_at DateTime  DEFAULT now()
		)
		ENGINE = ReplacingMergeTree
		ORDER BY (country, name)`
	if err := s.conn.Exec(ctx, schema); err != nil {
		return fmt.Errorf("create table: %w", err)
	}
	return nil
}

// InsertCities записывает города батчем.
// Батч — родной способ записи в ClickHouse: одна команда на ВСЕ строки,
// а не по запросу на город. Построчная вставка — главный антипаттерн CH.
func (s *Store) InsertCities(ctx context.Context, cities []geo.City) error {
	// PrepareBatch открывает поток вставки. Обратите внимание:
	// колонка id в списке НЕ указана — она MATERIALIZED, база посчитает сама.
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO cities (name, region, country, lat, lon)")
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}
	for _, c := range cities {
		// Append кладёт ОДНУ строку в батч — значения строго по порядку колонок
		if err := batch.Append(c.Name, c.Region, c.Country, c.Lat, c.Lon, c.CreatedAt); err != nil {
			return fmt.Errorf("append %q: %w", c.Name, err)
		}
	}
	return batch.Send()
}

// List возвращает города, отсортированные по стране и названию.
func (s *Store) List(ctx context.Context, limit int) ([]geo.City, error) {
	rows, err := s.conn.Query(ctx,
		"SELECT id, name, region, country, lat, lon, created_at FROM cities WHERE deleted = 0 ORDER BY country, name LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query cities: %w", err)
	}
	defer rows.Close() // закрываем курсор в любом исходе

	cities := make([]geo.City, 0, limit)
	for rows.Next() {
		var c geo.City
		// Scan раскладывает колонки текущей строки в поля структуры —
		// порядок аргументов обязан совпадать с порядком в SELECT
		if err := rows.Scan(&c.ID, &c.Name, &c.Region, &c.Country, &c.Lat, &c.Lon, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan city: %w", err)
		}
		cities = append(cities, c)
	}
	// rows.Err — проверка на ошибку, случившуюся В ПРОЦЕССЕ итерации:
	// разрыв сети посреди выборки выглядит как преждевременный конец,
	// без этой проверки мы бы молча вернули обрезанный список
	return cities, rows.Err()
}

// InsertCity добавляет один город и возвращает его с вычисленным ID.
func (s *Store) InsertCity(ctx context.Context, in geo.CreateCityInput) (geo.City, error) {
	c := geo.City{
		Name:      in.Name,
		Region:    in.Region,
		Country:   in.Country,
		Lat:       in.Lat,
		Lon:       in.Lon,
		CreatedAt: time.Now().UTC(),
	}

	// Батч из одной строки — редкий случай, когда построчная вставка
	// оправдана: это единичная пользовательская операция, а не поток.
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO cities (name, region, country, lat, lon, created_at)")
	if err != nil {
		return c, fmt.Errorf("prepare batch: %w", err)
	}
	if err := batch.Append(c.Name, c.Region, c.Country, c.Lat, c.Lon, c.CreatedAt); err != nil {
		return c, fmt.Errorf("append: %w", err)
	}
	if err := batch.Send(); err != nil {
		return c, fmt.Errorf("send batch: %w", err)
	}

	// ID вычисляет сама база (MATERIALIZED cityHash64) — достаём его
	// по натуральному ключу. RETURNING у ClickHouse нет, это не OLTP-база.
	row := s.conn.QueryRow(ctx,
		"SELECT id FROM cities WHERE name = ? AND lat = ? AND lon = ? AND deleted = 0",
		c.Name, c.Lat, c.Lon,
	)
	if err := row.Scan(&c.ID); err != nil {
		return c, fmt.Errorf("fetch id: %w", err)
	}
	return c, nil
}

// DeleteCity — мягкое удаление: ставит флаг deleted = 1.
// Возвращает false, если живого города с таким id не нашлось.
func (s *Store) DeleteCity(ctx context.Context, id uint64) (bool, error) {
	// Сначала честная проверка существования — иначе после любого
	// DELETE мы бы возвращали «true» даже для несуществующего id.
	var found uint64
	err := s.conn.QueryRow(ctx,
		"SELECT id FROM cities WHERE id = ? AND deleted = 0 LIMIT 1",
		id,
	).Scan(&found)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil // города нет — это НЕ ошибка, а 404
		}
		return false, fmt.Errorf("lookup: %w", err)
	}

	// Само удаление — UPDATE флага. SETTINGS mutations_sync = 1
	// заставляет базу дождаться применения мутации перед ответом:
	// без этого следующий SELECT мог бы ещё увидеть город живым
	// (мутации в CH по умолчанию асинхронные).
	err = s.conn.Exec(ctx,
		"ALTER TABLE cities UPDATE deleted = 1 WHERE id = ? SETTINGS mutations_sync = 1",
		id,
	)
	if err != nil {
		return false, fmt.Errorf("mark deleted: %w", err)
	}
	return true, nil
}

// Nearest возвращает до limit ближайших к точке городов.
// radiusKm = 0 означает «без ограничения по радиусу».
func (s *Store) Nearest(ctx context.Context, lat, lon float64, limit int, radiusKm float64) ([]geo.NearestCity, error) {
	// geoDistance(lon, lat, ...) — долгота ПЕРВАЯ, и это единственная пара
	// плейсхолдеров в SELECT. Аргументы собираем строго в порядке их
	// появления в тексте запроса — привязка позиционная, не по именам.
	query := "SELECT id, name, region, country, lat, lon, created_at, " +
		"geoDistance(lon, lat, ?, ?) / 1000 AS distance_km " +
		"FROM cities WHERE deleted = 0"

	args := []any{lon, lat} // аргументы для SELECT'ного geoDistance

	// Алиас distance_km доступен в WHERE — особенность ClickHouse.
	// Радиус в км, geoDistance вернул метры: делим на 1000 при вычислении.
	if radiusKm > 0 {
		query += " AND distance_km <= ?"
		args = append(args, radiusKm)
	}

	query += " ORDER BY distance_km ASC LIMIT ?"
	args = append(args, limit)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query nearest: %w", err)
	}
	defer rows.Close()

	out := make([]geo.NearestCity, 0, limit)
	for rows.Next() {
		var nc geo.NearestCity
		if err := rows.Scan(&nc.ID, &nc.Name, &nc.Region, &nc.Country,
			&nc.Lat, &nc.Lon, &nc.CreatedAt, &nc.DistanceKm); err != nil {
			return nil, fmt.Errorf("scan nearest: %w", err)
		}
		out = append(out, nc)
	}
	return out, rows.Err()
}
