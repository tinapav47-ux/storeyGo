# Используем официальный образ Go как базовый для сборки
FROM golang:1.22 AS builder

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем go.mod и go.sum
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download

# Копируем исходный код
COPY . .

# Компилируем приложение для arm64 (или arm для 32-битного Raspberry Pi)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o storeygo main.go

# Финальный образ на основе Playwright
FROM mcr.microsoft.com/playwright:v1.50.1-jammy

# Устанавливаем необходимые зависимости для Playwright
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
RUN npx playwright install --with-deps --force

# Копируем скомпилированный бинарник
COPY --from=builder /app/storeygo /usr/local/bin/storeygo

# Команда для запуска приложения
CMD ["storeygo"]
