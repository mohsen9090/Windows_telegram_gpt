
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/joho/godotenv"
)

var (
    bot         *tgbotapi.BotAPI
    userStates  = make(map[int64]string)
    currentPath = make(map[int64]string)
    isGPTMode   = make(map[int64]bool)
    uploadDir   = "uploads"
)

// تابع کمکی برای تبدیل سایز فایل به فرمت خوانا
func formatSize(size int64) string {
    if size < 1024 {
        return fmt.Sprintf("%d B", size)
    } else if size < 1024*1024 {
        return fmt.Sprintf("%.1f KB", float64(size)/1024)
    } else if size < 1024*1024*1024 {
        return fmt.Sprintf("%.1f MB", float64(size)/1024/1024)
    }
    return fmt.Sprintf("%.1f GB", float64(size)/1024/1024/1024)
}

// ایجاد کیبورد اصلی
func createMainKeyboard() tgbotapi.InlineKeyboardMarkup {
    return tgbotapi.NewInlineKeyboardMarkup(
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData("💬 Chat GPT", "gpt_start"),
            tgbotapi.NewInlineKeyboardButtonData("❌ خروج از GPT", "gpt_exit"),
        ),
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData("📁 مدیریت فایل سرور", "server_files"),
            tgbotapi.NewInlineKeyboardButtonData("📂 فایل‌های آپلودی", "uploaded_files"),
        ),
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData("❔ راهنما", "help"),
        ),
    )
}

// ایجاد کیبورد مدیریت فایل
func createFileManagerKeyboard(path string, isServerMode bool) tgbotapi.InlineKeyboardMarkup {
    var buttons [][]tgbotapi.InlineKeyboardButton

    // دکمه برگشت
    if path != "" && path != uploadDir {
        buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData("⬅️ برگشت", "back:"+filepath.Dir(path)),
        ))
    }

    // نمایش محتویات
    files, _ := ioutil.ReadDir(path)
    var rowButtons []tgbotapi.InlineKeyboardButton
    for _, file := range files {
        prefix := "📄 "
        if file.IsDir() {
            prefix = "📁 "
        }
        fullPath := filepath.Join(path, file.Name())
        
        button := tgbotapi.NewInlineKeyboardButtonData(
            prefix+file.Name(),
            "file:"+fullPath,
        )
        
        rowButtons = append(rowButtons, button)
        if len(rowButtons) == 2 {
            buttons = append(buttons, rowButtons)
            rowButtons = nil
        }
    }
    if len(rowButtons) > 0 {
        buttons = append(buttons, rowButtons)
    }

    // دکمه‌های عملیات
    if isServerMode {
        buttons = append(buttons,
            tgbotapi.NewInlineKeyboardRow(
                tgbotapi.NewInlineKeyboardButtonData("📁 پوشه جدید", "newdir:"+path),
                tgbotapi.NewInlineKeyboardButtonData("📄 فایل جدید", "newfile:"+path),
            ),
            tgbotapi.NewInlineKeyboardRow(
                tgbotapi.NewInlineKeyboardButtonData("📤 آپلود", "upload:"+path),
                tgbotapi.NewInlineKeyboardButtonData("❌ حذف", "delete:"+path),
            ),
        )
    }

    buttons = append(buttons,
        tgbotapi.NewInlineKeyboardRow(
            tgbotapi.NewInlineKeyboardButtonData("🔄 بروزرسانی", "refresh:"+path),
            tgbotapi.NewInlineKeyboardButtonData("🏠 منوی اصلی", "home"),
        ),
    )

    return tgbotapi.NewInlineKeyboardMarkup(buttons...)
}

// نمایش محتویات پوشه
func listDirectory(path string) (string, error) {
    files, err := ioutil.ReadDir(path)
    if err != nil {
        return "", err
    }

    var output strings.Builder
    output.WriteString(fmt.Sprintf("📂 مسیر فعلی: %s\n\n", path))

    var dirs []string
    var filesList []string

    for _, file := range files {
        if file.IsDir() {
            dirs = append(dirs, fmt.Sprintf("📁 %s/", file.Name()))
        } else {
            filesList = append(filesList, fmt.Sprintf("📄 %s (%s)", 
                file.Name(), formatSize(file.Size())))
        }
    }

    if len(dirs) > 0 {
        output.WriteString("📁 پوشه‌ها:\n")
        output.WriteString(strings.Join(dirs, "\n"))
        output.WriteString("\n\n")
    }

    if len(filesList) > 0 {
        output.WriteString("📄 فایل‌ها:\n")
        output.WriteString(strings.Join(filesList, "\n"))
    }

    if len(dirs) == 0 && len(filesList) == 0 {
        output.WriteString("📭 این پوشه خالی است.")
    }

    return output.String(), nil
}

// ارسال به GPT
func sendToGPT(text string) (string, error) {
    url := "https://openrouter.ai/api/v1/chat/completions"
    payload := map[string]interface{}{
        "model": "openai/gpt-3.5-turbo",
        "messages": []map[string]string{
            {"role": "user", "content": text},
        },
    }

    jsonData, err := json.Marshal(payload)
    if err != nil {
        return "", err
    }

    req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        return "", err
    }

    req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("HTTP-Referer", "https://github.com/yourusername")
    req.Header.Set("X-Title", "File Manager Bot")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        Choices []struct {
            Message struct {
                Content string `json:"content"`
            } `json:"message"`
        } `json:"choices"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }

    if len(result.Choices) > 0 {
        return result.Choices[0].Message.Content, nil
    }
    return "", fmt.Errorf("no response from GPT")
}

// مدیریت آپلود فایل
func handleFileUpload(update tgbotapi.Update) {
    chatID := update.Message.Chat.ID
    state, exists := userStates[chatID]
    if !exists || !strings.HasPrefix(state, "waiting_upload:") {
        return
    }

    path := strings.TrimPrefix(state, "waiting_upload:")
    var fileID string
    var fileName string

    if update.Message.Document != nil {
        fileID = update.Message.Document.FileID
        fileName = update.Message.Document.FileName
    } else if update.Message.Photo != nil && len(update.Message.Photo) > 0 {
        photos := update.Message.Photo
        fileID = photos[len(photos)-1].FileID
        fileName = fmt.Sprintf("photo_%d.jpg", time.Now().Unix())
    }

    if fileID != "" {
        file, err := bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
        if err != nil {
            bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در دریافت فایل"))
            return
        }

        resp, err := http.Get(file.Link(bot.Token))
        if err != nil {
            bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در دانلود فایل"))
            return
        }
        defer resp.Body.Close()

        targetPath := filepath.Join(path, fileName)
        out, err := os.Create(targetPath)
        if err != nil {
            bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در ذخیره فایل"))
            return
        }
        defer out.Close()

        _, err = io.Copy(out, resp.Body)
        if err != nil {
            bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در ذخیره فایل"))
            return
        }

        delete(userStates, chatID)
        msg := tgbotapi.NewMessage(chatID, "✅ فایل با موفقیت آپلود شد!")
        msg.ReplyMarkup = createFileManagerKeyboard(path, true)
        bot.Send(msg)
    }
}

// مدیریت کالبک‌ها
func handleCallback(update tgbotapi.Update) {
    query := update.CallbackQuery
    chatID := query.Message.Chat.ID
    data := query.Data

    parts := strings.SplitN(data, ":", 2)
    action := parts[0]
    var path string
    if len(parts) > 1 {
        path = parts[1]
    }

    switch action {
    case "gpt_start":
        isGPTMode[chatID] = true
        msg := tgbotapi.NewMessage(chatID, "✅ حالت Chat GPT فعال شد!\n🤖 پیام خود را بنویسید...")
        bot.Send(msg)

    case "gpt_exit":
        delete(isGPTMode, chatID)
        msg := tgbotapi.NewMessage(chatID, "✅ از حالت Chat GPT خارج شدید!")
        msg.ReplyMarkup = createMainKeyboard()
        bot.Send(msg)

    case "server_files":
        msg := tgbotapi.NewMessage(chatID, "📂 مدیریت فایل‌های سرور:")
        msg.ReplyMarkup = createFileManagerKeyboard("/", true)
        bot.Send(msg)

    case "uploaded_files":
        msg := tgbotapi.NewMessage(chatID, "📂 فایل‌های آپلود شده:")
        msg.ReplyMarkup = createFileManagerKeyboard(uploadDir, false)
        bot.Send(msg)

    case "file":
        if stat, err := os.Stat(path); err == nil {
            if stat.IsDir() {
                content, err := listDirectory(path)
                if err != nil {
                    bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در خواندن محتویات پوشه"))
                    return
                }
                msg := tgbotapi.NewMessage(chatID, content)
                msg.ReplyMarkup = createFileManagerKeyboard(path, !strings.HasPrefix(path, uploadDir))
                bot.Send(msg)
            } else {
                // ارسال فایل با نوع مناسب
                ext := strings.ToLower(filepath.Ext(path))
                switch ext {
                case ".jpg", ".jpeg", ".png", ".gif":
                    photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(path))
                    bot.Send(photo)
                case ".mp4", ".avi", ".mkv":
                    video := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(path))
                    bot.Send(video)
                case ".mp3", ".wav", ".ogg":
                    audio := tgbotapi.NewAudio(chatID, tgbotapi.FilePath(path))
                    bot.Send(audio)
                default:
                    doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(path))
                    bot.Send(doc)
                }

                // نمایش اطلاعات فایل
                info := fmt.Sprintf(`
📄 نام فایل: %s
📦 حجم: %s
📅 تاریخ ویرایش: %s
`,
                    stat.Name(),
                    formatSize(stat.Size()),
                    stat.ModTime().Format("2006-01-02 15:04:05"),
                )
                msg := tgbotapi.NewMessage(chatID, info)
                bot.Send(msg)
            }
        }

    case "back":
        content, err := listDirectory(path)
        if err != nil {
            bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در خواندن محتویات پوشه"))
            return
        }
        msg := tgbotapi.NewMessage(chatID, content)
        msg.ReplyMarkup = createFileManagerKeyboard(path, !strings.HasPrefix(path, uploadDir))
        bot.Send(msg)

    case "refresh":
        content, err := listDirectory(path)
        if err != nil {
            bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در بروزرسانی"))
            return
        }
        msg := tgbotapi.NewMessage(chatID, content)
        msg.ReplyMarkup = createFileManagerKeyboard(path, !strings.HasPrefix(path, uploadDir))
        bot.Send(msg)

    case "home":
        msg := tgbotapi.NewMessage(chatID, "🏠 منوی اصلی")
        msg.ReplyMarkup = createMainKeyboard()
        bot.Send(msg)

    case "newdir":
        userStates[chatID] = "waiting_mkdir:" + path
        msg := tgbotapi.NewMessage(chatID, "📁 نام پوشه جدید را وارد کنید:")
        bot.Send(msg)

    case "newfile":
        userStates[chatID] = "waiting_touch:" + path
        msg := tgbotapi.NewMessage(chatID, "📄 نام فایل جدید را وارد کنید:")
        bot.Send(msg)

    case "upload":
        userStates[chatID] = "waiting_upload:" + path
        msg := tgbotapi.NewMessage(chatID, "📤 فایل مورد نظر را ارسال کنید:")
        bot.Send(msg)

    case "delete":
        msg := tgbotapi.NewMessage(chatID, "⚠️ آیا از حذف این مورد اطمینان دارید؟")
        keyboard := tgbotapi.NewInlineKeyboardMarkup(
            tgbotapi.NewInlineKeyboardRow(
                tgbotapi.NewInlineKeyboardButtonData("✅ بله", "confirm_delete:"+path),
                tgbotapi.NewInlineKeyboardButtonData("❌ خیر", "cancel_delete:"+path),
            ),
        )
        msg.ReplyMarkup = keyboard
        bot.Send(msg)

    case "confirm_delete":
        err := os.RemoveAll(path)
        if err != nil {
            bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در حذف فایل/پوشه"))
            return
        }
        msg := tgbotapi.NewMessage(chatID, "✅ با موفقیت حذف شد!")
        parentDir := filepath.Dir(path)
        msg.ReplyMarkup = createFileManagerKeyboard(parentDir, !strings.HasPrefix(parentDir, uploadDir))
        bot.Send(msg)

    case "cancel_delete":
        msg := tgbotapi.NewMessage(chatID, "❌ عملیات حذف لغو شد.")
        msg.ReplyMarkup = createFileManagerKeyboard(filepath.Dir(path), !strings.HasPrefix(path, uploadDir))
        bot.Send(msg)

    case "help":
        helpText := `
📚 راهنمای دستورات:

🤖 Chat GPT:
• شروع گفتگو با دکمه Chat GPT
• خروج با دکمه خروج از GPT

📁 مدیریت فایل سرور:
• مرور و مدیریت فایل‌های سرور
• ایجاد پوشه و فایل جدید
• آپلود و دانلود فایل
• حذف فایل و پوشه

📂 فایل‌های آپلودی:
• مشاهده فایل‌های آپلود شده
• دانلود فایل‌ها
• حذف فایل‌ها

⚡️ دستورات:
/start - شروع مجدد ربات
/help - نمایش این راهنما`

        msg := tgbotapi.NewMessage(chatID, helpText)
        msg.ReplyMarkup = createMainKeyboard()
        bot.Send(msg)
    }
}

// مدیریت ورودی کاربر
func handleUserInput(update tgbotapi.Update) {
    chatID := update.Message.Chat.ID
    state, exists := userStates[chatID]
    if !exists {
        return
    }

    if strings.HasPrefix(state, "waiting_mkdir:") {
        path := strings.TrimPrefix(state, "waiting_mkdir:")
        newPath := filepath.Join(path, update.Message.Text)
        err := os.MkdirAll(newPath, 0755)
        if err != nil {
            bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در ایجاد پوشه"))
            return
        }

        delete(userStates, chatID)
        msg := tgbotapi.NewMessage(chatID, "✅ پوشه با موفقیت ایجاد شد!")
        msg.ReplyMarkup = createFileManagerKeyboard(path, true)
        bot.Send(msg)

    } else if strings.HasPrefix(state, "waiting_touch:") {
        path := strings.TrimPrefix(state, "waiting_touch:")
        newPath := filepath.Join(path, update.Message.Text)
        _, err := os.Create(newPath)
        if err != nil {
            bot.Send(tgbotapi.NewMessage(chatID, "❌ خطا در ایجاد فایل"))
            return
        }

        delete(userStates, chatID)
        msg := tgbotapi.NewMessage(chatID, "✅ فایل با موفقیت ایجاد شد!")
        msg.ReplyMarkup = createFileManagerKeyboard(path, true)
        bot.Send(msg)
    }
}

func main() {
    // خواندن تنظیمات از فایل .env
    err := godotenv.Load()
    if err != nil {
        log.Fatal("Error loading .env file")
    }

    // ساخت پوشه آپلود
    os.MkdirAll(uploadDir, 0755)

    // راه‌اندازی ربات
    bot, err = tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("🤖 ربات با نام %s راه‌اندازی شد!\n", bot.Self.UserName)

    updateConfig := tgbotapi.NewUpdate(0)
    updateConfig.Timeout = 60

    updates := bot.GetUpdatesChan(updateConfig)

    for update := range updates {
        if update.CallbackQuery != nil {
            handleCallback(update)
        } else if update.Message != nil {
            chatID := update.Message.Chat.ID

            if update.Message.IsCommand() {
                if update.Message.Command() == "start" {
                    msg := tgbotapi.NewMessage(chatID, "👋 خوش آمدید!\nلطفاً یکی از گزینه‌های زیر را انتخاب کنید:")
                    msg.ReplyMarkup = createMainKeyboard()
                    bot.Send(msg)
                    continue
                }
            } else if update.Message.Document != nil || update.Message.Photo != nil {
                handleFileUpload(update)
            } else if update.Message.Text != "" {
                if isGPTMode[chatID] {
                    response, err := sendToGPT(update.Message.Text)
                    if err != nil {
                        msg := tgbotapi.NewMessage(chatID, "❌ خطا در ارتباط با GPT")
                        bot.Send(msg)
                        continue
                    }
                    msg := tgbotapi.NewMessage(chatID, response)
                    bot.Send(msg)
                } else {
                    handleUserInput(update)
                }
            }
        }
    }
}

