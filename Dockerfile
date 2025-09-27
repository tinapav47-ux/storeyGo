# Используем официальный golang образ
FROM golang:1.22-bullseye

# Устанавливаем зависимости для Playwright Chromium
RUN apt-get update && \
    apt-get install -y wget gnupg libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 libxcomposite1 libxrandr2 libxdamage1 libx11-xcb1 libxshmfence1 libxrender1 libgbm1 libpango-1.0-0 libasound2 libwoff1 fonts-liberation libxcb-dri3-0 libxkbcommon0 libxfixes3 libxi6 curl && \
    rm -rf /var/lib/apt/lists/*

# Устанавливаем Playwright CLI для установки браузеров
RUN go install github.com/playwright-community/playwright-go/cmd/playwright@latest

# Создаем рабочую директорию
WORKDIR /app

# Копируем файлы проекта
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Скачиваем браузеры для Playwright
RUN playwright install

# Компилируем Go-приложение
RUN go build -o storeybot main.go

# Экспортируем порт (для бота необязательно)
EXPOSE 8080

# Запуск бота
CMD ["./storeybot"]
