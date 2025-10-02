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
    && rm -rf /var/lib/apt/lists/*

# Установка Node.js
RUN curl -fsSL https://deb.nodesource.com/setup_18.x | bash - && \
    apt-get install -y nodejs

# Установка конкретной версии Playwright (1.50.1)
RUN npm install -g playwright@1.50.1
RUN npx playwright install-deps
RUN npx playwright install chromium

WORKDIR /app
COPY --from=builder /app/storeygo /app/storeygo

CMD ["/app/storeygo"]
