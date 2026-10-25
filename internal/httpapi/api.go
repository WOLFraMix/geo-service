package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"geo-service/internal/geo"
	"geo-service/internal/storage"
)

// API — держит зависимости хендлеров (store) и собирает маршруты.
// Такой приём называется dependency injection: хендлеры не создают
// соединение с базой сами, а получают его снаружи — легко тестировать.
type API struct {
	store *storage.Store
}

// New создаёт API и регистрирует ВСЕ маршруты на переданном роутере.
func New(mux *http.ServeMux, store *storage.Store) *API {
	a := &API{store: store}

	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /cities", a.listCities)
	mux.HandleFunc("POST /cities", a.createCity)
	mux.HandleFunc("DELETE /cities/{id}", a.deleteCity)
	mux.HandleFunc("GET /cities/nearest", a.nearestCities)

	return a
}

// health — проверка живости.
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// listCities — GET /cities — список городов.
func (a *API) listCities(w http.ResponseWriter, r *http.Request) {
	// query-параметр limit; дефолт 50, максимум 500 — защита от «дай всё»
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "limit must be 1..500")
			return
		}
		limit = n
	}

	cities, err := a.store.List(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cities)
}

// createCity — POST /cities — добавление города.
// Тело запроса — JSON без id и created_at: их генерирует сервер/база.
func (a *API) createCity(w http.ResponseWriter, r *http.Request) {
	// Ограничиваем размер тела: без этого злоумышленник
	// может прислать гигабайт JSON
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

	var in geo.CreateCityInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Валидация — обязательный ритуал перед записью в базу
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	city, err := a.store.InsertCity(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 201 Created — REST-канон для успешного создания
	writeJSON(w, http.StatusCreated, city)
}

// deleteCity — DELETE /cities/{id}.
// {id} — паттерн Go 1.22+: значение достаём через r.PathValue("id").
func (a *API) deleteCity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be unsigned integer")
		return
	}

	deleted, err := a.store.DeleteCity(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		// Не нашли — честный 404, а не молчаливый 200
		writeError(w, http.StatusNotFound, "city not found")
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204: удалено, тела нет
}

// nearestCities — GET /cities/nearest?lat=..&lon=..&limit=..&radius_km=..
// Тот самый гаверсинус из пакета geo — наконец в деле.
func (a *API) nearestCities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// lat/lon обязательны и обязаны быть числами
	lat, err1 := strconv.ParseFloat(q.Get("lat"), 64)
	lon, err2 := strconv.ParseFloat(q.Get("lon"), 64)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "lat and lon are required numbers")
		return
	}
	// Диапазоны географии: широта ±90, долгота ±180
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		writeError(w, http.StatusBadRequest, "lat must be -90..90, lon -180..180")
		return
	}

	limit := 10 // дефолт
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusBadRequest, "limit must be 1..100")
			return
		}
		limit = n
	}

	radius := 0.0 // 0 = без ограничения радиуса
	if v := q.Get("radius_km"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 {
			writeError(w, http.StatusBadRequest, "radius_km must be non-negative number")
			return
		}
		radius = f
	}

	cities, err := a.store.Nearest(r.Context(), lat, lon, limit, radius)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cities)
}

// writeJSON — сериализация ответа. Единая точка для всех ответов:
// поменять заголовки или формат достаточно в одном месте.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError — единый формат ошибок {"error": "..."}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
