# Используем официальный Playwright образ как базовый
# Он уже содержит браузеры и зависимости
FROM mcr.microsoft.com/playwright:v1.45.0-focal AS base

# Установим Go внутри этого образа
RUN apt-get update && apt-get install -y --no-install-recommends \
    golang-go git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Копируем модули
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники
COPY main.go ./

# Собираем бинарь
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o insta-bot main.go

# Финальный образ (можно использовать тот же base)
FROM mcr.microsoft.com/playwright:v1.45.0-focal

WORKDIR /app

COPY --from=base /app/insta-bot /usr/local/bin/insta-bot

ENV TELEGRAM_TOKEN=""

ENTRYPOINT ["insta-bot"]
