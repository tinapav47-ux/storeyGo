# storeyGo — Instagram Stories Downloader Bot (Go + Playwright)

Telegram-бот, который скачивает истории (stories) через `insta-stories.ru` и отправляет пользователю.

>>⚠️ Важно: Этот бот предназначен только для личного использования и обучения.
>>Telegram-бот переписан на Go для экспериментов с производительностью и использованием ресурсов. Первоначальная версия на Python была ресурсоёмкой.

---

## Структура

- `main.go` — основной код бота  
- `go.mod` / `go.sum` — зависимости Go  
- `Dockerfile` — сборка образа с Playwright и приложением  
- `README.md` — инструкция и документация  

---

## Используемые версии

- Go: `1.21` (arm64, Raspberry Pi)  
- [playwright-go](https://github.com/playwright-community/playwright-go): `v0.5001.0`  
- [go-telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api): `v5.5.1`  

---

## Команды в termimal

- github.com/playwright-community/playwright-go v0.5001.0
>>go list -m all | Select-String playwright
-  github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
>>go list -m all | Select-String telegram


## Пользование

- Бот работает через Telegram (текущая реализация). По желанию можно настроить использование через terminal.
- Для запуска бота на своём устройстве нужен собственный токен Telegram, полученный у @BotFather.
- Доступ к моему боту не прдеоставляется по личным соображениям безопасности.
- Этот проект предназначен исключительно для личного обучения и экспериментов, а не для публичного использования.

---

## Безопасность

- Токен Telegram задаётся через переменные окружения, чтобы не хранить его в коде или в Git.

---

## Дополнительно

- Исходно бот был на Python и потреблял много ресурсов.
- Переписанный на Go бот показывает меньшую нагрузку и позволяет изучать производительность.
- Docker обеспечивает изоляцию и упрощает запуск на Raspberry Pi.
