# ----------- Stage 1: Build Go binary -----------
FROM golang:1.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o storeygo main.go

# ----------- Stage 2: Runtime -----------
FROM ubuntu:22.04

# Устанавливаем базовые зависимости + xz-utils для распаковки Node.js
RUN apt-get update && apt-get install -y \
    wget curl ca-certificates bash coreutils xz-utils \
    fonts-freefont-ttf libnss3 libxss1 libasound2 libxtst6 libgtk-3-0 libgbm-dev \
    && rm -rf /var/lib/apt/lists/*

# Установка Go
RUN curl -fsSL https://go.dev/dl/go1.22.8.linux-arm64.tar.gz | tar -C /usr/local -xz
ENV PATH=$PATH:/usr/local/go/bin:/root/go/bin

# Установка Node.js 18 (LTS) вручную для ARM64
RUN curl -fsSL https://nodejs.org/dist/v18.20.1/node-v18.20.1-linux-arm64.tar.xz | tar -xJ -C /usr/local --strip-components=1

# Проверка версий (опционально)
RUN node -v && npm -v

# Переменная для Playwright браузеров
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

WORKDIR /app

# Установка Playwright 1.50.1
RUN npm install -g playwright@1.50.1 \
    && npx playwright@1.50.1 install --with-deps chromium

# Установка playwright-go CLI
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@v0.5001.0 \
    && /root/go/bin/playwright install

# Копируем собранный Go бинарь
COPY --from=builder /app/storeygo /app/storeygo

# Используем shell form для подстановки переменной
CMD /app/storeygo -token $TELEGRAM_TOKEN
