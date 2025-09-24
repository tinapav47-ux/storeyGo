package main

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "mime"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    playwright "github.com/playwright-community/playwright-go"
)

var (
    BASE_SITE           = "https://insta-stories.ru"
    TIMEOUT_GOTO        = 30 * time.Second
    TIMEOUT_WAIT_STORY  = 10 * time.Second
    userAgents = []string{
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/116.0.5845.97 Safari/537.36",
        "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 Version/16.6 Mobile/15E148 Safari/604.1",
    }
)

// MediaItem описывает одну медиа-единицу (картинка или видео)
type MediaItem struct {
    Type       string // "image" или "video"
    URL        string
    StoryIndex int
    Order      int
}

func randomUA() string {
    return userAgents[int(time.Now().UnixNano())%len(userAgents)]
}

func isValidHTTP(u string) bool {
    if u == "" {
        return false
    }
    return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// fetchMediaLinks — рендерит страницу через Playwright и собирает ссылки медиа
func fetchMediaLinks(ctx context.Context, pw *playwright.Playwright, username string, sendMsg func(string)) ([]MediaItem, error) {
    url := fmt.Sprintf("%s/ru/%s", BASE_SITE, username)

    browser, err := pw.Chromium.Launch()
    if err != nil {
        return nil, fmt.Errorf("launch browser: %w", err)
    }
    defer browser.Close()

    ua := randomUA()
    contextOpts := playwright.BrowserNewContextOptions{
        UserAgent: playwright.String(ua),
    }
    bctx, err := browser.NewContext(contextOpts)
    if err != nil {
        return nil, fmt.Errorf("new context: %w", err)
    }
    page, err := bctx.NewPage()
    if err != nil {
        return nil, fmt.Errorf("new page: %w", err)
    }

    // Переход к странице
    _, err = page.Goto(url, playwright.PageGotoOptions{Timeout: playwright.Float(float64(TIMEOUT_GOTO / time.Millisecond))})
    if err != nil {
        return nil, fmt.Errorf("goto %s: %w", url, err)
    }

    // Проверка на сообщение в div.tab-content p.text-center
    sel, err := page.QuerySelector("div.tab-content p.text-center")
    if err == nil && sel != nil {
        txt, _ := sel.InnerText()
        txt = strings.TrimSpace(txt)
        if txt != "" {
            sendMsg(txt)
            return nil, nil
        }
    }

    // Ждём появления сторис
    err = page.WaitForSelector(".story", playwright.PageWaitForSelectorOptions{
        Timeout: playwright.Float(float64(TIMEOUT_WAIT_STORY / time.Millisecond)),
    })
    if err != nil {
        // возможно просто нет сторис
        return nil, fmt.Errorf("no .story elements: %w", err)
    }

    stories, err := page.QuerySelectorAll(".story")
    if err != nil {
        return nil, fmt.Errorf("querySelectorAll .story: %w", err)
    }

    var mediaFound []MediaItem
    idx := 1

    for i, s := range stories {
        mediaBox, _ := s.QuerySelector(".mediaBox")
        if mediaBox == nil {
            continue
        }

        mediaBlock, _ := mediaBox.QuerySelector(".media")
        if mediaBlock != nil {
            // попытка нажать Play, если видео
            btn, _ := mediaBlock.QuerySelector(`button[aria-label="Play video"]`)
            if btn != nil {
                _ = btn.Click(playwright.PageClickOptions{Force: playwright.Bool(true)})
                time.Sleep(2 * time.Second)
            }
            // поиск source[type="video/mp4"]
            sourceEl, _ := mediaBlock.QuerySelector(`source[type="video/mp4"]`)
            if sourceEl == nil {
                sourceEl, _ = s.QuerySelector(`source[type="video/mp4"]`)
            }
            if sourceEl != nil {
                src, _ := sourceEl.GetAttribute("src")
                typ, _ := sourceEl.GetAttribute("type")
                if src != "" && strings.TrimSpace(strings.ToLower(typ)) == "video/mp4" {
                    mediaFound = append(mediaFound, MediaItem{
                        Type:       "video",
                        URL:        src,
                        StoryIndex: i + 1,
                        Order:      idx,
                    })
                    idx++
                    continue
                }
            }
        }

        // пробуем картинку
        imgEl, _ := mediaBox.QuerySelector("img")
        if imgEl != nil {
            src, _ := imgEl.GetAttribute("src")
            if src != "" {
                mediaFound = append(mediaFound, MediaItem{
                    Type:       "image",
                    URL:        src,
                    StoryIndex: i + 1,
                    Order:      idx,
                })
                idx++
                continue
            }
        }
    }

    return mediaFound, nil
}

// downloadFile загружает файл по url и сохраняет в папке folder как index.ext
func downloadFile(client *http.Client, url, folder string, index int, mtype string) (string, error) {
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("User-Agent", randomUA())
    resp, err := client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return "", fmt.Errorf("bad status: %d", resp.StatusCode)
    }

    ext := ""
    if ct := resp.Header.Get("Content-Type"); ct != "" {
        if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
            ext = exts[0]
        }
    }
    if ext == "" {
        ext = filepath.Ext(url)
    }
    if ext == "" {
        if mtype == "image" {
            ext = ".jpg"
        } else {
            ext = ".mp4"
        }
    }
    filename := fmt.Sprintf("%d%s", index, ext)
    fullpath := filepath.Join(folder, filename)

    f, err := os.Create(fullpath)
    if err != nil {
        return "", err
    }
    defer f.Close()

    _, err = io.Copy(f, resp.Body)
    if err != nil {
        return "", err
    }
    return fullpath, nil
}

func prepareFolder(folder string) error {
    return os.MkdirAll(folder, os.ModePerm)
}

// sendMedia отправляет медиафайлы: фото группами (по 10) и видео по отдельности
func sendMedia(bot *tgbotapi.BotAPI, chatID int64, folder string) error {
    files, err := ioutil.ReadDir(folder)
    if err != nil {
        return err
    }

    var images [][]byte
    var imageNames []string
    type vid struct {
        Name string
        Data []byte
    }
    var videos []vid

    for _, f := range files {
        if f.IsDir() {
            continue
        }
        path := filepath.Join(folder, f.Name())
        data, err := ioutil.ReadFile(path)
        if err != nil {
            log.Printf("Ошибка чтения файла %s: %v", path, err)
            continue
        }
        ext := strings.ToLower(filepath.Ext(f.Name()))
        if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
            images = append(images, data)
            imageNames = append(imageNames, f.Name())
        } else if ext == ".mp4" || ext == ".mov" {
            videos = append(videos, vid{Name: f.Name(), Data: data})
        }
    }

    // отправка фото группами по 10
    const maxGroup = 10
    for i := 0; i < len(images); i += maxGroup {
        j := i + maxGroup
        if j > len(images) {
            j = len(images)
        }
        media := []interface{}{}
        for k := i; k < j; k++ {
            media = append(media, tgbotapi.NewInputMediaPhoto(tgbotapi.FileBytes{
                Name:  imageNames[k],
                Bytes: images[k],
            }))
        }
        msg := tgbotapi.MediaGroupConfig{
            ChatID: chatID,
            Media:  media,
        }
        if _, err := bot.SendMediaGroup(msg); err != nil {
            log.Printf("SendMediaGroup error: %v", err)
        }
    }

    // отправка видео по отдельности
    for _, v := range videos {
        msg := tgbotapi.NewVideo(chatID, tgbotapi.FileBytes{
            Name:  v.Name,
            Bytes: v.Data,
        })
        if _, err := bot.Send(msg); err != nil {
            log.Printf("send video error: %v", err)
        }
    }

    // финальное сообщение
    _, _ = bot.Send(tgbotapi.NewMessage(chatID, "Done."))

    // удаляем папку
    _ = os.RemoveAll(folder)
    return nil
}

func workerDownload(ctx context.Context, client *http.Client, jobs <-chan MediaItem, results chan<- string, folder string, wg *sync.WaitGroup) {
    defer wg.Done()
    for item := range jobs {
        if !isValidHTTP(item.URL) {
            results <- ""
            continue
        }
        path, err := downloadFile(client, item.URL, folder, item.Order, item.Type)
        if err != nil {
            log.Printf("download error %s: %v", item.URL, err)
            results <- ""
        } else {
            results <- path
        }
    }
}

func main() {
    token := os.Getenv("TELEGRAM_TOKEN")
    if token == "" {
        log.Fatal("TELEGRAM_TOKEN не задана")
    }
    bot, err := tgbotapi.NewBotAPI(token)
    if err != nil {
        log.Fatalf("Создать бота не удалось: %v", err)
    }

    u := tgbotapi.NewUpdate(0)
    u.Timeout = 60
    updates := bot.GetUpdatesChan(u)

    waiting := sync.Map{}

    pw, err := playwright.Run()
    if err != nil {
        log.Fatalf("playwright.Run: %v", err)
    }
    defer pw.Stop()

    for update := range updates {
        if update.Message == nil {
            continue
        }
        chatID := update.Message.Chat.ID

        if update.Message.IsCommand() {
            if update.Message.Command() == "view" {
                waiting.Store(chatID, true)
                bot.Send(tgbotapi.NewMessage(chatID, "Send Instagram username:"))
            } else {
                bot.Send(tgbotapi.NewMessage(chatID, "Unknown command."))
            }
            continue
        }

        if update.Message.Text != "" {
            _, ok := waiting.Load(chatID)
            if !ok {
                bot.Send(tgbotapi.NewMessage(chatID, "Press /view."))
                continue
            }
            waiting.Delete(chatID)
            username := strings.TrimSpace(update.Message.Text)
            bot.Send(tgbotapi.NewMessage(chatID, "Please wait."))

            go func(username string, chatID int64) {
                ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
                defer cancel()

                sendToChat := func(s string) {
                    bot.Send(tgbotapi.NewMessage(chatID, s))
                }

                media, err := fetchMediaLinks(ctx, pw, username, sendToChat)
                if err != nil {
                    log.Printf("fetchMediaLinks: %v", err)
                    bot.Send(tgbotapi.NewMessage(chatID, "Error: "+err.Error()))
                    return
                }
                if len(media) == 0 {
                    bot.Send(tgbotapi.NewMessage(chatID, "No media found."))
                    return
                }

                folder := fmt.Sprintf("%s_%d", username, time.Now().Unix())
                if err := prepareFolder(folder); err != nil {
                    bot.Send(tgbotapi.NewMessage(chatID, "Folder creation error: "+err.Error()))
                    return
                }

                client := &http.Client{Timeout: 120 * time.Second}

                jobs := make(chan MediaItem, len(media))
                results := make(chan string, len(media))
                var wg sync.WaitGroup
                wc := 6
                if wc > len(media) {
                    wc = len(media)
                }
                for i := 0; i < wc; i++ {
                    wg.Add(1)
                    go workerDownload(ctx, client, jobs, results, folder, &wg)
                }
                for _, m := range media {
                    jobs <- m
                }
                close(jobs)
                wg.Wait()
                close(results)

                count := 0
                for p := range results {
                    if p != "" {
                        count++
                    }
                }
                log.Printf("[DONE] downloaded: %d files", count)
                if err := sendMedia(bot, chatID, folder); err != nil {
                    log.Printf("sendMedia error: %v", err)
                    bot.Send(tgbotapi.NewMessage(chatID, "Send media error: "+err.Error()))
                }
            }(username, chatID)
        }
    }
}
