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

# Устанавливаем Go
RUN curl -fsSL https://go.dev/dl/go1.22.8.linux-arm64.tar.gz | tar -C /usr/local -xz
ENV PATH=$PATH:/usr/local/go/bin:/root/go/bin

# Устанавливаем Node.js
RUN curl -fsSL https://deb.nodesource.com/setup_18.x | bash - \
    && apt-get install -y nodejs

# Переменная для Playwright браузеров
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

WORKDIR /app

# Установка Playwright 1.50.1
RUN npm install -g playwright@1.50.1 \
    && npx playwright@1.50.1 install --with-deps chromium

# Установка playwright-go CLI (версия, совместимая с 1.50.1)
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@v0.5001.0 \
    && /root/go/bin/playwright install

# Копируем собранный Go бинарь
COPY --from=builder /app/storeygo /app/storeygo

# Запуск бота с токеном из переменной окружения
CMD ["/app/storeygo", "-token", "${TELEGRAM_TOKEN}"]
