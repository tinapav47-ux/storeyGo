# ----------- Stage 1: Build Go binary -----------
FROM golang:1.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o storeygo main.go

# ----------- Stage 2: Runtime -----------
FROM ubuntu:22.04

# Базовые пакеты: curl + зависимости для Playwright
RUN apt-get update && apt-get install -y \
    wget curl ca-certificates \
    bash coreutils \
    fonts-freefont-ttf libnss3 libxss1 libasound2 libxtst6 libgtk-3-0 libgbm-dev \
    nodejs npm \
    && rm -rf /var/lib/apt/lists/*

# Установка Playwright
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

RUN npm install -g playwright@1.50.1 \
    && npx playwright@1.50.1 install --with-deps chromium

# Установка Go-пакета playwright-go
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@v0.5001.0 \
    && /root/go/bin/playwright install

WORKDIR /app

# Копируем бинарь из builder
COPY --from=builder /app/storeygo /app/storeygo

# Запуск с использованием переменной окружения
CMD ["/bin/bash", "-c", "/app/storeygo -token $TELEGRAM_TOKEN"]
