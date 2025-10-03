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
	timeoutWait = 15 * time.Second // Увеличено для видео
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
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

	// Ожидание полной загрузки страницы (networkidle — когда нет сетевых запросов)
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.String("networkidle"),
		Timeout: playwright.Float(10000), // 10 секунд
	}); err != nil {
		log.Printf("[DEBUG] WaitForLoadState timeout for username %s: %v", username, err)
		// Продолжаем выполнение
	}

	// Проверка на сообщение "У пользователя нет историй" (таймаут 10 сек)
	textEl, err := page.WaitForSelector("div.tab-content p.text-center", playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(10000), // 10 секунд
	})
	if err != nil {
		log.Printf("[DEBUG] No text-center element found for username %s: %v", username, err)
	} else if textEl != nil {
		message, _ := textEl.InnerText()
		message = strings.TrimSpace(message)
		if message != "" {
			bot.Send(tgbotapi.NewMessage(chatID, message)) // Сообщение от сайта
			return nil, nil
		} else {
			log.Printf("[DEBUG] Text-center element found but empty for username %s", username)
		}
	}

	// Проверка на наличие историй
	if _, err := page.WaitForSelector(".story", playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(float64(timeoutWait.Milliseconds())),
	}); err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "У пользователя нет историй"))
		return nil, nil // Медиа не найдено
	}

	stories, err := page.QuerySelectorAll(".story")
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("[ERROR] Error fetching stories: %v", err)))
		return nil, err
	}

	var found []map[string]string
	for i, story := range stories {
		mediaBox, err := story.QuerySelector(".mediaBox")
		if err != nil || mediaBox == nil {
			log.Printf("[DEBUG] No mediaBox found for story %d: %v", i+1, err)
			continue
		}

		mediaBlock, err := mediaBox.QuerySelector(".media")
		if err != nil || mediaBlock == nil {
			log.Printf("[DEBUG] No mediaBlock found for story %d: %v", i+1, err)
			continue
		}

		// Попытка клика на кнопку воспроизведения
		btn, err := mediaBlock.QuerySelector(`button[aria-label="Play video"]`)
		if err != nil {
			log.Printf("[DEBUG] No play button found for story %d: %v", i+1, err)
		} else if btn != nil {
			if err := btn.Click(playwright.ElementHandleClickOptions{Force: playwright.Bool(true)}); err != nil {
				log.Printf("[DEBUG] Failed to click play button for story %d: %v", i+1, err)
			} else {
				log.Printf("[DEBUG] Clicked play button for story %d", i+1)
				page.WaitForTimeout(5000) // Ожидание загрузки видео
			}
		}

		// Поиск видео
		sourceEl, err := mediaBlock.QuerySelector(`source[type="video/mp4"]`)
		if err != nil || sourceEl == nil {
			log.Printf("[DEBUG] No video source found in mediaBlock for story %d: %v", i+1, err)
			sourceEl, err = story.QuerySelector(`source[type="video/mp4"]`)
			if err != nil || sourceEl == nil {
				log.Printf("[DEBUG] No video source found in story for story %d: %v", i+1, err)
				// Альтернативный селектор для видео
				sourceEl, err = story.QuerySelector(`video source`)
				if err != nil || sourceEl == nil {
					log.Printf("[DEBUG] No video source found with alternative selector for story %d: %v", i+1, err)
				}
			}
		}

		if sourceEl != nil {
			src, err := sourceEl.GetAttribute("src")
			if err != nil || src == "" {
				log.Printf("[DEBUG] No valid video src for story %d: %v", i+1, err)
			} else {
				log.Printf("[DEBUG] Found video for story %d: %s", i+1, src)
				found = append(found, map[string]string{
					"type":       "video",
					"url":        src,
					"storyIndex": fmt.Sprintf("%d", i+1),
				})
				continue
			}
		}

		// Поиск изображения
		imgEl, err := mediaBox.QuerySelector("img")
		if err != nil || imgEl == nil {
			log.Printf("[DEBUG] No image found for story %d: %v", i+1, err)
			continue
		}

		src, err := imgEl.GetAttribute("src")
		if err != nil || src == "" {
			log.Printf("[DEBUG] No valid image src for story %d: %v", i+1, err)
			continue
		}

		log.Printf("[DEBUG] Found image for story %d: %s", i+1, src)
		found = append(found, map[string]string{
			"type":       "image",
			"url":        src,
			"storyIndex": fmt.Sprintf("%d", i+1),
		})
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

			// Если медиа не найдено, отправляем сообщение
			if len(media) == 0 {
				bot.Send(tgbotapi.NewMessage(chatID, "У пользователя нет историй"))
				continue
			}

			// Создание папки для сохранения файлов
			folder := username
			var mediaFiles []interface{} // Для фото и видео

			// Скачивание файлов
			for i, item := range media {
				url := item["url"]
				mtype := item["type"]
				ext := "jpg"
				if mtype == "video" {
					ext = "mp4"
				}
				filename := fmt.Sprintf("%d.%s", i+1, ext)

				// Скачивание
				if err := saveFile(url, folder, filename); err != nil {
					log.Printf("[DEBUG] Failed to download %s: %v", url, err)
					continue
				}

				filePath := filepath.Join(folder, filename)

				// Проверка размера файла (ограничение Telegram: 50 МБ)
				fileInfo, err := os.Stat(filePath)
				if err != nil || fileInfo.Size() > 50*1024*1024 {
					log.Printf("[DEBUG] File %s is too large or inaccessible", filename)
					os.Remove(filePath) // Удаляем большой файл
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
				log.Printf("[DEBUG] Failed to remove folder %s: %v", folder, err)
			}
		}
	}
}
