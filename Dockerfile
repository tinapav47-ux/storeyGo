# ----------- Stage 1: Build Go binary -----------
FROM golang:1.22 AS builder

WORKDIR /app

# Скачиваем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь код и собираем бинарь
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o storeygo main.go

# ----------- Stage 2: Runtime -----------
FROM ubuntu:22.04

# Устанавливаем базовые зависимости
RUN apt-get update && apt-get install -y \
    wget curl ca-certificates \
    fonts-freefont-ttf libnss3 libxss1 libasound2 libxtst6 libgtk-3-0 libgbm-dev \
    && rm -rf /var/lib/apt/lists/*

# Устанавливаем Node.js
RUN curl -fsSL https://deb.nodesource.com/setup_18.x | bash - \
    && apt-get install -y nodejs

# Переменная для Playwright браузеров
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

WORKDIR /app

# Установка Playwright 1.50.1
RUN npm install -g playwright@1.50.1 \
    && npx playwright@1.50.1 install --with-deps chromium

# Установка playwright-go CLI
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@latest

# Копируем собранный Go бинарь
COPY --from=builder /app/storeygo /app/storeygo

# Запуск бота
CMD ["/app/storeygo"]
