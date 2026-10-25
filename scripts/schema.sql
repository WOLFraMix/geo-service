CREATE TABLE IF NOT EXISTS cities
(
    -- MATERIALIZED — вычислимая колонка: значение не передаётся при вставке,
    -- ClickHouse сам считает cityHash64(name, lat, lon) на лету.
    -- Это даёт нам детерминированный ID: одинаковый город всегда
    -- получает один и тот же id, без генераторов на стороне приложения.
    id          UInt64    MATERIALIZED cityHash64(name, lat, lon),
    name        String,
    region      String,
    country     String,
    lat         Float64,  -- широта, градусы
    lon         Float64,  -- долгота, градусы
    created_at  DateTime  DEFAULT now()
)
ENGINE = ReplacingMergeTree
-- ORDER BY — это не просто сортировка, а первичный ключ хранения:
-- данные на диске физически упорядочены по нему, и запросы с фильтром
-- по этим колонкам читают только нужные куски колонок (это и есть
-- главная магия колоночных баз — но об этом подробнее на поиске ближайших)
ORDER BY (country, name)