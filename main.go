package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/playwright-community/playwright-go"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.97 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.97 Safari/537.36",
	"Mozilla/5.0 (Linux; Android 13; Pixel 7 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.98 Mobile Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1",
}

const (
	baseSite    = "https://insta-stories.ru"
	timeoutGoto = 30 * time.Second
	timeoutWait = 10 * time.Second
)

func getRandomUserAgent() string {
	rand.Seed(time.Now().UnixNano())
	return userAgents[rand.Intn(len(userAgents))]
}

// validateInstagramUsername проверяет, соответствует ли username правилам Instagram
func validateInstagramUsername(username string) bool {
	regex := regexp.MustCompile(`^[a-zA-Z0-9._]{1,30}$`)
	return regex.MatchString(username) && !strings.ContainsAny(username, " ")
}

func saveFile(url, folder, filename string) error {
	if err := os.MkdirAll(folder, os.ModePerm); err != nil {
		return err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", getRandomUserAgent())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath.Join(folder, filename))
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func fetchMediaLinks(username string, bot *tgbotapi.BotAPI, chatID int64) ([]map[string]string, error) {
	pw, err := playwright.Run(&playwright.RunOptions{
		Verbose: true,
	})
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("[ERROR] Failed to start Playwright: %v", err)))
		return nil, err
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("[ERROR] Failed to launch browser: %v", err)))
		return nil, err
	}
	defer browser.Close()

	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		UserAgent: playwright.String(getRandomUserAgent()),
	})
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("[ERROR] Failed to create page: %v", err)))
		return nil, err
	}

	url := fmt.Sprintf("%s/ru/%s", baseSite, username)
	if _, err := page.Goto(url, playwright.PageGotoOptions{
		Timeout: playwright.Float(float64(timeoutGoto.Milliseconds())),
	}); err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("[ERROR] Failed to load page: %v", err)))
		return nil, err
	}

	textEl, err := page.WaitForSelector("div.tab-content p.text-center", playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(10000),
	})
	if err != nil {
		// таймаут — элемента нет, продолжаем
	} else if textEl != nil {
		message, _ := textEl.InnerText()
		message = strings.TrimSpace(message)
		if message != "" {
			bot.Send(tgbotapi.NewMessage(chatID, message)) // Сообщение от сайта
			return nil, nil
		}
	}

	if _, err := page.WaitForSelector(".story", playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(float64(timeoutWait.Milliseconds())),
	}); err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "No stories found"))
		return nil, nil // Медиа не найдено — ничего не отправляем
	}

	stories, err := page.QuerySelectorAll(".story")
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("[ERROR] Error fetching stories: %v", err)))
		return nil, err
	}

	var found []map[string]string
	for i, story := range stories {
		mediaBox, _ := story.QuerySelector(".mediaBox")
		if mediaBox == nil {
			continue
		}

		mediaBlock, _ := mediaBox.QuerySelector(".media")
		if mediaBlock != nil {
			btn, _ := mediaBlock.QuerySelector(`button[aria-label="Play video"]`)
			if btn != nil {
				btn.Click(playwright.ElementHandleClickOptions{Force: playwright.Bool(true)})
				page.WaitForTimeout(5000)
			}

			sourceEl, _ := mediaBlock.QuerySelector(`source[type="video/mp4"]`)
			if sourceEl == nil {
				sourceEl, _ = story.QuerySelector(`source[type="video/mp4"]`)
			}

			if sourceEl != nil {
				src, _ := sourceEl.GetAttribute("src")
				if src != "" {
					found = append(found, map[string]string{
						"type":       "video",
						"url":        src,
						"storyIndex": fmt.Sprintf("%d", i+1),
					})
					continue
				}
			}
		}

		imgEl, _ := mediaBox.QuerySelector("img")
		if imgEl != nil {
			src, _ := imgEl.GetAttribute("src")
			if src != "" {
				found = append(found, map[string]string{
					"type":       "image",
					"url":        src,
					"storyIndex": fmt.Sprintf("%d", i+1),
				})
				continue
			}
		}
	}

	return found, nil
}

func main() {
	// Парсинг токена из флага командной строки
	tokenPtr := flag.String("token", "", "Telegram Bot Token")
	flag.Parse()

	if *tokenPtr == "" {
		log.Fatal("Error: Telegram Bot Token is required. Use -token flag.")
	}

	// Инициализация Telegram-бота с повторными попытками
	var bot *tgbotapi.BotAPI
	var err error
	for i := 0; i < 5; i++ {
		log.Printf("[INFO] Attempting to initialize Telegram bot (attempt %d/5)...", i+1)
		bot, err = tgbotapi.NewBotAPI(*tokenPtr)
		if err == nil {
			break
		}
		log.Printf("[ERROR] Failed to initialize Telegram bot: %v, retrying in 5 seconds...", err)
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		log.Fatalf("[ERROR] Failed to initialize Telegram bot after 5 attempts: %v", err)
	}

	bot.Debug = true // Включаем отладку
	log.Printf("[INFO] Bot %s started", bot.Self.UserName)

	// Настройка обновлений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Карта для хранения состояния ожидания username
	waitingForUsername := make(map[int64]bool)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		chatID := update.Message.Chat.ID
		username := strings.TrimSpace(update.Message.Text)

		// Проверка команды /view
		if update.Message.Text == "/view" {
			waitingForUsername[chatID] = true
			bot.Send(tgbotapi.NewMessage(chatID, "Sent username:"))
			continue
		}

		// Если бот ожидает username
		if waitingForUsername[chatID] {
			// Проверка формата username
			if !validateInstagramUsername(username) {
				bot.Send(tgbotapi.NewMessage(chatID, "Invalid username format. Use only Latin letters, numbers, underscores, and dots."))
				waitingForUsername[chatID] = false
				continue
			}

			// Сброс состояния
			waitingForUsername[chatID] = false

			// Отправляем "Please wait"
			bot.Send(tgbotapi.NewMessage(chatID, "Please wait"))

			// Получение медиа
			media, err := fetchMediaLinks(username, bot, chatID)
			if err != nil {
				bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("[ERROR] %v", err)))
				continue
			}

			if len(media) == 0 {
				continue // Ничего не отправляем
			}

			// Создание папки для сохранения файлов
			folder := username
			var mediaFiles []interface{} // Для фото и видео

			for i, item := range media {
				url := item["url"]
				mtype := item["type"]
				ext := "jpg"
				if mtype == "video" {
					ext = "mp4"
				}
				filename := fmt.Sprintf("%d.%s", i+1, ext)

				// Процесс скачивания в фоне
				if err := saveFile(url, folder, filename); err != nil {
					log.Printf("[DEBUG] Failed to download %s: %v", url, err)
					continue
				}

				filePath := filepath.Join(folder, filename)

				// Проверка размера файла (ограничение Telegram: 50 МБ)
				fileInfo, err := os.Stat(filePath)
				if err != nil || fileInfo.Size() > 50*1024*1024 {
					log.Printf("[DEBUG] File %s is too large or inaccessible", filename)
					continue
				}

				// Добавление файла в медиагруппу
				if mtype == "image" {
					mediaFiles = append(mediaFiles, tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(filePath)))
				} else if mtype == "video" {
					mediaFiles = append(mediaFiles, tgbotapi.NewInputMediaVideo(tgbotapi.FilePath(filePath)))
				}
			}

			// Отправка медиагруппы
			if len(mediaFiles) > 0 {
				mediaGroup := tgbotapi.NewMediaGroup(chatID, mediaFiles)
				_, err := bot.SendMediaGroup(mediaGroup)
				if err != nil {
					bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("[ERROR] Failed to send media: %v", err)))
				} else {
					bot.Send(tgbotapi.NewMessage(chatID, "Done."))
				}
			}

			// Удаление папки
			if err := os.RemoveAll(folder); err != nil {
				log.Printf("Failed to remove folder %s: %v", folder, err)
			}
		}
	}
}

