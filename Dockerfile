FROM golang:1.22 as builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o storeygo main.go

FROM ubuntu:22.04

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

# Очистка кэша npm и удаление любых существующих версий Playwright
RUN npm cache clean --force && \
    npm uninstall -g playwright || true && \
    npm install -g playwright@1.50.1 && \
    npx playwright@1.50.1 install --with-deps chromium

# Установка playwright-go (библиотека) и CLI-инструмента
RUN go install github.com/playwright-community/playwright-go@v0.5001.0
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@v0.5001.0

# Установка драйвера Playwright 1.50.1 через CLI
RUN /go/bin/playwright install --with-deps chromium

COPY --from=builder /app/storeygo /app/storeygo

CMD ["/app/storeygo"]

