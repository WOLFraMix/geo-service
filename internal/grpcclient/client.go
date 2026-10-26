package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure" // без TLS: локальная разработка
	"google.golang.org/grpc/status"

	"geo-service/internal/geo"
	pb "geo-service/internal/pb"
)

// Resolver — обёртка над gRPC-соединением с резолвером.
type Resolver struct {
	conn *grpc.ClientConn      // соединение (для закрытия при shutdown)
	stub pb.CityResolverClient // сгенерированная клиентская заглушка
}

// New устанавливает соединение. insecure — без TLS: для локальной сети
// и разработки; в проде сюда пришла бы credentials.TransportCredentials.
func New(addr string) (*Resolver, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	return &Resolver{conn: conn, stub: pb.NewCityResolverClient(conn)}, nil
}

// Close освобождает соединение — вызовем при graceful shutdown.
func (r *Resolver) Close() error {
	return r.conn.Close()
}

// Nearest проксирует запрос к резолверу и переводит ответ в доменную модель.
func (r *Resolver) Nearest(ctx context.Context, lat, lon float64, limit int, radiusKm float64) ([]geo.NearestCity, error) {
	// Таймаут именно на сетевой вызов: HTTP-запрос клиента к API не должен
	// умирать из-за медленного резолвера без ограничений
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := r.stub.Nearest(ctx, &pb.NearestRequest{
		Lat:      lat,
		Lon:      lon,
		Limit:    int32(limit),
		RadiusKm: radiusKm,
	})
	if err != nil {
		return nil, status.Convert(err).Err() // gRPC-статус → обычная ошибка
	}

	out := make([]geo.NearestCity, 0, len(resp.Cities))
	for _, c := range resp.Cities {
		created, _ := time.Parse(time.RFC3339, c.GetCreatedAt())
		out = append(out, geo.NearestCity{
			ID:         c.GetId(),
			Name:       c.GetName(),
			Region:     c.GetRegion(),
			Country:    c.GetCountry(),
			Lat:        c.GetLat(),
			Lon:        c.GetLon(),
			CreatedAt:  created,
			DistanceKm: c.GetDistanceKm(),
		})
	}
	return out, nil
}
