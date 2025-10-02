# Используем официальный образ Go как базовый
FROM golang:1.22 AS builder

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем go.mod и go.sum (если они есть)
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download

# Копируем исходный код
COPY . .

# Компилируем приложение
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o storeygo main.go

# Финальный образ на основе минимального образа с Playwright
FROM mcr.microsoft.com/playwright:v1.44.0-jammy

# Устанавливаем необходимые зависимости для Playwright
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
RUN npx playwright install --with-deps

# Копируем скомпилированный бинарник из предыдущего этапа
COPY --from=builder /app/storeygo /usr/local/bin/storeygo

# Команда для запуска приложения
CMD ["storeygo"]



