package geo

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// City — сущность справочника.
// Латиница в тегах json — как поле будет выглядеть в HTTP-ответах.
type City struct {
	ID        uint64    `json:"id"`         // внутренний идентификатор
	Name      string    `json:"name"`       // название, например "Астрахань"
	Region    string    `json:"region"`     // регион/область
	Country   string    `json:"country"`    // страна
	Lat       float64   `json:"lat"`        // широта, градусы
	Lon       float64   `json:"lon"`        // долгота, градусы
	CreatedAt time.Time `json:"created_at"` // когда добавлен
}

// NearestCity — город + расстояние до точки поиска.
// Встроенная структура geo.City: её поля «разворачиваются» внутрь
// при JSON-сериализации, ответ будет плоским: {id, name, ..., distance_km}.
type NearestCity struct {
	City
	DistanceKm float64 `json:"distance_km"`
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

// CreateCityInput — то, что клиент присылает в POST /cities.
// id и created_at клиент не задаёт — они генерируются на нашей стороне.
type CreateCityInput struct {
	Name    string  `json:"name"`
	Region  string  `json:"region"`
	Country string  `json:"country"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
}

// Validate — проверка входных данных. Возвращает первую найденную ошибку.
func (in *CreateCityInput) Validate() error {
	switch {
	case in.Name == "":
		return errors.New("name is required")
	case in.Country == "":
		return errors.New("country is required")
	case in.Lat < -90 || in.Lat > 90:
		return errors.New("lat must be -90..90")
	case in.Lon < -180 || in.Lon > 180:
		return errors.New("lon must be -180..180")
	}
	return nil
}
