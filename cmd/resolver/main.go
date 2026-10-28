package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

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

	// TCP-листенер: gRPC поверх HTTP/2,
	// но в Go всё равно слушаем TCP
	lis, err := net.Listen("tcp", cfg.GRPCListen)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	// gRPC-сервер и регистрация нашего сервиса
	grpcSrv := grpc.NewServer()
	pb.RegisterCityResolverServer(grpcSrv, resolver.New(store))

	// Reflection: без него инструменты не смогут «опрашивать» сервис
	reflection.Register(grpcSrv)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("resolver listening on %s", cfg.GRPCListen)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-stop
	log.Println("shutdown signal received")

	// GracefulStop дорабатывает текущие RPC и лишь потом закрывает
	// соединения — gRPC-версия той же вежливости
	grpcSrv.GracefulStop()
	store.Close()
	log.Println("bye")
}
