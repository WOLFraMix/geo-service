# === Этап 1: сборка ===
# Полноценный образ с тулчейном Go — только для компиляции
FROM golang:1.27.3-alpine3.24 AS builder

# git нужен для go mod download, ca-certificates — для HTTPS
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Сначала копируем только файлы зависимостей и качаем модули:
# этот слой закэшируется, и при изменении кода кэш не сгорит —
# повторная сборка будет секундной, а не минутной
COPY go.mod go.sum ./
RUN go mod download

# Потом код и сборка. CGO_ENABLED=0 — статический бинарник
# без libc: сможет жить в пустом окружении. -trimpath и -ldflags
# убирают из бинаря локальные пути — меньше размер, больше воспроизводимость
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w" \
    -o /out/app ./cmd/${TARGET}

# === Этап 2: рантайм ===
# Минимальный образ: в нём нет Go, компилятора и вообще почти ничего —
# только то, что нужно бинарю (и shell для отладки; нужен совсем пустой — берите scratch)
FROM alpine:3.24

# Сертификаты: наш код ходит по сети, а gRPC в проде — это TLS
RUN apk add --no-cache ca-certificates

COPY --from=builder /out/app /app

EXPOSE 8080

ENTRYPOINT ["/app"]