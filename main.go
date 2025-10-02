package main

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/playwright-community/playwright-go"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.97 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.97 Safari/537.36",
	"Mozilla/5.0 (Linux; Android 13; Pixel 7 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.98 Mobile Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1",
}

const (
	baseSite              = "https://insta-stories.ru"
	timeoutGoto           = 30 * time.Second
	timeoutWait           = 10 * time.Second
	maxConcurrentRequests = 10  // Максимум одновременных запросов к Playwright/боту
	sendQueueBuffer       = 100 // размер очереди отправки сообщений

)

var (
	waitingForUsername = map[int64]bool{} // chat_id -> ждем username
	semaphore          = make(chan struct{}, maxConcurrentRequests)
	sendQueue          = make(chan func(), sendQueueBuffer) // очередь отправки сообщений

)

func getRandomUserAgent() string {
	rand.Seed(time.Now().UnixNano())
	return userAgents[rand.Intn(len(userAgents))]
}

// saveFile скачивает медиа в папку
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

// fetchMediaLinks парсит сторис и возвращает ссылки на картинки и видео
func fetchMediaLinks(username string) ([]map[string]string, string, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, "", err
	}
	defer pw.Stop()

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		return nil, "", err
	}
	defer browser.Close()

	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		UserAgent: playwright.String(getRandomUserAgent()),
	})
	if err != nil {
		return nil, "", err
	}

	url := fmt.Sprintf("%s/ru/%s", baseSite, username)
	fmt.Println("[INFO] Загружаю страницу:", url)

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		Timeout: playwright.Float(float64(timeoutGoto.Milliseconds())),
	}); err != nil {
		return nil, "", err
	}

	// Проверка текста p.text-center
	textEl, _ := page.WaitForSelector("div.tab-content p.text-center", playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(5000),
	})
	if textEl != nil {
		message, _ := textEl.InnerText()
		message = strings.TrimSpace(message)
		if message != "" {
			return nil, message, nil
		}
	}

	// Берём только первый блок актуальных историй
	container, err := page.QuerySelector("div.stories-container")
	if err != nil || container == nil {
		return nil, "No relevant stories found", nil
	}

	stories, err := container.QuerySelectorAll("div.story")
	if err != nil {
		return nil, "", err
	}

	if len(stories) == 0 {
		return nil, "The user has no current stories.", nil
	}

	var found []map[string]string
	for i, story := range stories {
		mediaBox, _ := story.QuerySelector(".mediaBox")
		if mediaBox == nil {
			continue
		}

		// Видео
		mediaBlock, _ := mediaBox.QuerySelector(".media")
		if mediaBlock != nil {
			btn, _ := mediaBlock.QuerySelector(`button[aria-label="Play video"]`)
			if btn != nil {
				btn.Click(playwright.ElementHandleClickOptions{Force: playwright.Bool(true)})
				page.WaitForTimeout(3000)
			}

			sourceEl, _ := mediaBlock.QuerySelector(`source[type="video/mp4"]`)
			if sourceEl == nil {
				// Ждём до 5 секунд пока появится
				sourceEl, _ = mediaBlock.WaitForSelector(`source[type="video/mp4"]`, playwright.ElementHandleWaitForSelectorOptions{
					Timeout: playwright.Float(5000),
					State:   playwright.WaitForSelectorStateAttached, // <- правильный тип
				})
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

		// Картинка
		imgEl, _ := mediaBox.QuerySelector("img")
		if imgEl != nil {
			src, _ := imgEl.GetAttribute("src")
			if src != "" {
				found = append(found, map[string]string{
					"type":       "image",
					"url":        src,
					"storyIndex": fmt.Sprintf("%d", i+1),
				})
			}
		}
	}

	return found, "", nil
}

func mediaSender(bot *tgbotapi.BotAPI) {
	for job := range sendQueue {
		job()
	}
}

// sendMedia отправляет медиа в Telegram и удаляет папку после отправки
func sendMedia(bot *tgbotapi.BotAPI, chatID int64, folder string) {
	files, _ := os.ReadDir(folder)
	var media []interface{}
	count := 0

	for _, f := range files {
		path := filepath.Join(folder, f.Name())
		if strings.HasSuffix(f.Name(), ".jpg") || strings.HasSuffix(f.Name(), ".png") {
			photo := tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(path))
			media = append(media, photo)
			count++
			if count == 10 {
				bot.SendMediaGroup(tgbotapi.MediaGroupConfig{
					ChatID: chatID,
					Media:  media,
				})
				media = nil
				count = 0
			}
		}
	}
	if len(media) > 0 {
		tempMedia := media
		sendQueue <- func() {
			bot.SendMediaGroup(tgbotapi.MediaGroupConfig{
				ChatID: chatID,
				Media:  tempMedia,
			})
		}
	}

	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".mp4") {
			path := filepath.Join(folder, f.Name())
			tempPath := path
			sendQueue <- func() {
				video := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(tempPath))
				bot.Send(video)
			}
		}
	}

	sendQueue <- func() {
		bot.Send(tgbotapi.NewMessage(chatID, "Done."))
	}

	// Удаляем папку со всеми файлами
	sendQueue <- func() {
		if err := os.RemoveAll(folder); err != nil {
			fmt.Printf("[ERROR] Не удалось удалить папку %s: %v\n", folder, err)
		} else {
			fmt.Printf("[INFO] Папка %s успешно удалена\n", folder)
		}
	}
}

func main() {

	    // Установка драйвера Playwright
    err := playwright.Install(&playwright.RunOptions{
        SkipInstallBrowsers: true, // Браузеры уже установлены
    })
    if err != nil {
        fmt.Printf("Failed to install playwright driver: %v\n", err)
    }
	
	botToken := os.Getenv("TELEGRAM_TOKEN")
if botToken == "" {
    panic("TELEGRAM_TOKEN environment variable is not set")
}

bot, err := tgbotapi.NewBotAPI(botToken)
if err != nil {
    panic(err)
}


	go mediaSender(bot) // запускаем горутину очереди отправки

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		chatID := update.Message.Chat.ID
		text := strings.TrimSpace(update.Message.Text)

		// --- Обработка команды /view ---
		if text == "/view" {
			waitingForUsername[chatID] = true
			bot.Send(tgbotapi.NewMessage(chatID, "Sent username:"))
			continue
		}

		// --- Если ждем username от пользователя ---
		if waitingForUsername[chatID] {
			username := text
			// ----------- Добавляем проверку на латиницу ---------
			validUsername := regexp.MustCompile(`^[A-Za-z0-9._]+$`)
			if !validUsername.MatchString(username) {
				bot.Send(tgbotapi.NewMessage(chatID, "Invalid username. Use only Latin letters, numbers, periods, and underscores."))
				continue // останавливаем дальнейшую обработку
			}
			// ----------- Добавляем проверку на латиницу ---------

			waitingForUsername[chatID] = false // <- сбрасываем только после успешной проверки

			bot.Send(tgbotapi.NewMessage(chatID, "Processing, please wait..."))

			// --- Горутин с семафором ---
			go func(chatID int64, username string) {
				semaphore <- struct{}{}        // захватываем слот
				defer func() { <-semaphore }() // освобождаем слот после работы
				// --- Горутин с семафором ---

				media, message, err := fetchMediaLinks(username)
				if err != nil {
					sendQueue <- func() {
						bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Error: %v", err)))
					}
					return
				}

				if message != "" {
					sendQueue <- func() {
						bot.Send(tgbotapi.NewMessage(chatID, message))
					}
					return
				}

				if len(media) == 0 {
					sendQueue <- func() {
						bot.Send(tgbotapi.NewMessage(chatID, "Media not found"))
					}
					return
				}

				folder := username
				for i, item := range media {
					url := item["url"]
					mtype := item["type"]
					ext := "jpg"
					if mtype == "video" {
						ext = "mp4"
					}
					filename := fmt.Sprintf("%d.%s", i+1, ext)
					if err := saveFile(url, folder, filename); err != nil {
						sendQueue <- func() {
							bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("Failed to download %s", url)))
						}
					}
				}

				sendMedia(bot, chatID, folder)
			}(chatID, username)

			continue
		}
		// --- Если пользователь пишет что-то без /view ---
		bot.Send(tgbotapi.NewMessage(chatID, "First use the /view command and then enter username."))
	}
}

