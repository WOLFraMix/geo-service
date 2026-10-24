package geo

import (
	"fmt"
	"math"
	"time"
)

// City — сущность справочника.
// Латиница в тегах json — как поле будет выглядеть в HTTP-ответах.
type City struct {
	ID        int64     `json:"id"`         // внутренний идентификатор
	Name      string    `json:"name"`       // название, например "Астрахань"
	Region    string    `json:"region"`     // регион/область
	Country   string    `json:"country"`    // страна
	Lat       float64   `json:"lat"`        // широта, градусы
	Lon       float64   `json:"lon"`        // долгота, градусы
	CreatedAt time.Time `json:"created_at"` // когда добавлен
}

// Пакетная константа: радиус Земли в километрах.
const earthRadiusKm = 6371.0

// DistanceKm — расстояние между двумя точками по формуле гаверсинуса.
// Формула считает длину дуги большого круга на сфере — для городов
// точность более чем достаточная (погрешность < 0.5%).
func DistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	// Градусы в радианы: тригонометрия в Go работает только с радианами.
	rad := math.Pi / 180.0
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad

	// Гаверсинус центрального угла:
	// h = sin²(Δφ/2) + cos(φ1)·cos(φ2)·sin²(Δλ/2)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)

	// 2·R·asin(√h) — длина дуги.
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(a))
}

// NearestRequest — параметры поиска ближайших городов.
type NearestRequest struct {
	Lat   float64 // широта точки отсчёта
	Lon   float64 // долгота точки отсчёта
	Limit int     // сколько городов вернуть
	MaxKm float64 // максимальный радиус поиска (0 = без ограничения)
}

// Ошибка "город не найден" — её будут обрабатывать HTTP-хендлеры,
// превращая в 404 (как ErrorDocument в watcher'е, но по HTTP-канонам).
var ErrNotFound = fmt.Errorf("city not found")
