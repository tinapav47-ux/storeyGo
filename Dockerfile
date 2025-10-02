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

# Устанавливаем только системные зависимости и Chromium
RUN npx playwright install-deps
RUN npx playwright install chromium

COPY --from=builder /app/storeygo /app/storeygo

CMD ["/app/storeygo"]
