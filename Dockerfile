FROM golang:1.22 as builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o storeygo main.go

FROM ubuntu:22.04

# Установка системных зависимостей
RUN apt-get update && \
    apt-get install -y \
    wget \
    curl \
    ca-certificates \
    fonts-freefont-ttf \
    libnss3 \
    libxss1 \
    libasound2 \
    libxtst6 \
    libgtk-3-0 \
    libgbm-dev \
    && rm -rf /var/lib/apt/lists/*

# Установка Node.js
RUN curl -fsSL https://deb.nodesource.com/setup_18.x | bash - && \
    apt-get install -y nodejs

WORKDIR /app

# Установка Playwright 1.50.1 и драйвера
RUN npm install -g playwright@1.50.1
RUN npx playwright@1.50.1 install --with-deps chromium

COPY --from=builder /app/storeygo /app/storeygo

# Явно устанавливаем драйвер для playwright-go
RUN GO111MODULE=off go get github.com/playwright-community/playwright-go && \
    /go/bin/playwright-go install --force --version 1.50.1

CMD ["/app/storeygo"]
