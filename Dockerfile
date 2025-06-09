# Multi-stage build для оптимизации размера образа
FROM golang:1.21-alpine AS builder

# Установка необходимых пакетов
RUN apk add --no-cache git ca-certificates tzdata

# Установка рабочей директории
WORKDIR /app

# Копирование go.mod и go.sum для кэширования зависимостей
COPY go.mod go.sum ./

# Загрузка зависимостей
RUN go mod download

# Копирование исходного кода
COPY . .

# Сборка приложения
ARG VERSION=dev
ARG BUILD_TIME
ARG COMMIT_HASH

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=${VERSION} \
              -X main.buildTime=${BUILD_TIME} \
              -X main.commit=${COMMIT_HASH}" \
    -o streaming-server ./cmd/server

# Финальный образ
FROM alpine:latest

# Установка необходимых пакетов
RUN apk --no-cache add ca-certificates tzdata wget

# Создание пользователя для безопасности
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Установка рабочей директории
WORKDIR /app

# Копирование бинарника из builder stage
COPY --from=builder /app/streaming-server .

# Копирование конфигурационных файлов
COPY --from=builder /app/examples/config/ ./config/

# Создание директорий для сертификатов и логов
RUN mkdir -p /app/certs /app/logs && \
    chown -R appuser:appgroup /app

# Переключение на непривилегированного пользователя
USER appuser

# Открытие портов
EXPOSE 8080 8081 8082 8443 8444 8445

# Проверка здоровья
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Запуск приложения
CMD ["./streaming-server"]
