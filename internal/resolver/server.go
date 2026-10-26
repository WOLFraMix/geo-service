package resolver

import (
	"context"

	"google.golang.org/grpc/codes"  // коды ошибок gRPC (иные, чем HTTP)
	"google.golang.org/grpc/status" // оборачивает ошибки в код+сообщение

	pb "geo-service/internal/pb" // сгенерированный код
	"geo-service/internal/storage"
)

// Server реализует интерфейс CityResolverServer, сгенерированный из .proto.
// Встраивание UnimplementedCityResolverServer — канонический приём:
// когда в .proto добавятся новые методы, а мы их не реализовали,
// заглушка вернёт codes.Unimplemented вместо паники.
type Server struct {
	pb.UnimplementedCityResolverServer

	store *storage.Store // резолвер ходит в ClickHouse напрямую
}

// New собирает сервер с зависимостями.
func New(store *storage.Store) *Server {
	return &Server{store: store}
}

// Nearest — реализация RPC. Сигнатура задаётся .proto,
// изменишь контракт — компилятор тут же напомнит о несоответствии.
func (s *Server) Nearest(ctx context.Context, req *pb.NearestRequest) (*pb.NearestResponse, error) {
	// Валидация и на сервере: клиент может быть чужим или устаревшим.
	// codes.InvalidArgument — gRPC-аналог HTTP 400.
	if req.GetLimit() < 1 || req.GetLimit() > 100 {
		return nil, status.Error(codes.InvalidArgument, "limit must be 1..100")
	}
	if req.GetLat() < -90 || req.GetLat() > 90 || req.GetLon() < -180 || req.GetLon() > 180 {
		return nil, status.Error(codes.InvalidArgument, "lat must be -90..90, lon -180..180")
	}

	// Двусторонняя конвертация модель ↔ proto — плата за контракт.
	// В больших проектах это выносят в отдельные мапперы; у нас компактно.
	cities, err := s.store.Nearest(ctx, req.GetLat(), req.GetLon(), int(req.GetLimit()), req.GetRadiusKm())
	if err != nil {
		// Ошибка хранилища — не вина клиента: codes.Internal ≈ HTTP 500.
		return nil, status.Error(codes.Internal, "storage failure")
	}

	resp := &pb.NearestResponse{Cities: make([]*pb.City, 0, len(cities))}
	for _, c := range cities {
		resp.Cities = append(resp.Cities, &pb.City{
			Id:         c.ID,
			Name:       c.Name,
			Region:     c.Region,
			Country:    c.Country,
			Lat:        c.Lat,
			Lon:        c.Lon,
			CreatedAt:  c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), // RFC3339
			DistanceKm: c.DistanceKm,
		})
	}
	return resp, nil
}
