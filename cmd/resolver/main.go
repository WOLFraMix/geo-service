package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection" // позволяет grpcurl и Postman видеть методы сервиса

	"geo-service/internal/config"
	pb "geo-service/internal/pb"
	"geo-service/internal/resolver"
	"geo-service/internal/storage"
)

func main() {
	cfg := config.New()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	store, err := storage.New(ctx, cfg.CHAddr, cfg.CHPassword)
	if err != nil {
		log.Fatalf("connect to clickhouse: %v", err)
	}
	log.Printf("clickhouse connected: %s", cfg.CHAddr)

	// TCP-листенер: gRPC поверх HTTP/2, но в Go всё равно слушаем TCP
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	// gRPC-сервер и регистрация нашего сервиса
	grpcSrv := grpc.NewServer()
	pb.RegisterCityResolverServer(grpcSrv, resolver.New(store))

	// Reflection: без него инструменты не смогут «опрашивать» сервис
	reflection.Register(grpcSrv)

	log.Printf("resolver listening on :50051")
	log.Fatal(grpcSrv.Serve(lis)) // Serve блокируется до остановки
}
