FROM golang:1.22 as builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o storeygo main.go

FROM ubuntu:22.04

# Установка зависимостей
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

# Установка Node.js 18.x
RUN curl -fsSL https://deb.nodesource.com/setup_18.x | bash - && \
    apt-get install -y nodejs

# Создаем рабочую директорию
WORKDIR /app

# Создаем package.json и устанавливаем Playwright ГЛОБАЛЬНО
RUN npm init -y && \
    npm install -g playwright@1.55.1

# Устанавливаем системные зависимости для браузеров
RUN npx playwright install-deps

# Устанавливаем браузеры (только Chromium для экономии места)
RUN npx playwright install chromium

# Копируем бинарник Go
COPY --from=builder /app/storeygo /app/storeygo

# Проверяем что все установилось
RUN npx playwright --version && \
    echo "Playwright installed successfully"

CMD ["/app/storeygo"]
