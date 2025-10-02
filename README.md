# storeyGo — Instagram Stories Downloader Bot (Go + Playwright)

Telegram-бот, который скачивает истории (stories) через `insta-stories.ru` и отправляет пользователю.

## Структура

- `main.go` — основной код
- `go.mod` / `go.sum` — зависимости Go
- `Dockerfile` — образ на основе Playwright, включающий ваше приложение
- `.github/workflows/docker-image.yml` — CI: сборка и пуш Docker-образа
- `README.md` — инструкция

## Установка и запуск

### Локально (без Docker)

1. Убедитесь, что Playwright и браузеры установлены локально:

   ```bash
   npm i -g playwright
   playwright install --with-deps
