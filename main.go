package main

import (
    "archive/zip"
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/joho/godotenv"
)

type File struct {
    FileID   string `json:"file_id"`
    FileName string `json:"file_name"`
}

type Message struct {
    Text     string  `json:"text"`
    Chat     struct{ ID int64 } `json:"chat"`
    Document *File   `json:"document,omitempty"`
    Photo    []File  `json:"photo,omitempty"`
    Video    *File   `json:"video,omitempty"`
    Audio    *File   `json:"audio,omitempty"`
    Voice    *File   `json:"voice,omitempty"`
}

type Update struct {
    UpdateID int     `json:"update_id"`
    Message  Message `json:"message"`
}

var (
    token       string
    openaiKey   string
    apiURL      string
    offset      = 0
    userModules = make(map[int64]string)
    userStates  = make(map[int64]bool)
    uploadDir   string
    templateDir string
    bot         *tgbotapi.BotAPI
)

func sendMessage(chatID int64, text string) {
    msg := tgbotapi.NewMessage(chatID, text)
    bot.Send(msg)
}

func sendDocument(chatID int64, fileName, filePath string) {
    file, err := os.Open(filePath)
    if err != nil {
        sendMessage(chatID, "❌ Error opening file")
        return
    }
    defer file.Close()

    doc := tgbotapi.NewDocument(chatID, tgbotapi.FileReader{Name: fileName, Reader: file})
    _, err = bot.Send(doc)
    if err != nil {
        sendMessage(chatID, "❌ Error sending document")
    }
}

func getUpdates() []Update {
    url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=30", apiURL, offset+1)
    resp, err := http.Get(url)
    if err != nil {
        return nil
    }
    defer resp.Body.Close()

    var result struct {
        Ok     bool     `json:"ok"`
        Result []Update `json:"result"`
    }
    json.NewDecoder(resp.Body).Decode(&result)

    if len(result.Result) > 0 {
        offset = result.Result[len(result.Result)-1].UpdateID
    }

    return result.Result
}

func createZipArchive(sourceDir, targetFile string) error {
    zipfile, err := os.Create(targetFile)
    if err != nil {
        return err
    }
    defer zipfile.Close()

    archive := zip.NewWriter(zipfile)
    defer archive.Close()

    return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        header, err := zip.FileInfoHeader(info)
        if err != nil {
            return err
        }

        header.Name = strings.TrimPrefix(path, sourceDir)
        if info.IsDir() {
            header.Name += "/"
        }

        writer, err := archive.CreateHeader(header)
        if err != nil {
            return err
        }

        if !info.IsDir() {
            file, err := os.Open(path)
            if err != nil {
                return err
            }
            defer file.Close()
            _, err = io.Copy(writer, file)
        }
        return err
    })
}

func handleIncomingFile(update Update) {
    chatID := update.Message.Chat.ID
    module := userModules[chatID]

    if module == "" {
        sendMessage(chatID, "⚠️ Please activate a module first using /learn")
        return
    }

    var file *File
    var fileName string

    switch {
    case update.Message.Document != nil:
        file = update.Message.Document
        fileName = file.FileName
    case len(update.Message.Photo) > 0:
        file = &update.Message.Photo[len(update.Message.Photo)-1]
        fileName = fmt.Sprintf("photo%d.jpg", time.Now().Unix())
    case update.Message.Video != nil:
        file = update.Message.Video
        fileName = file.FileName
    case update.Message.Audio != nil:
        file = update.Message.Audio
        fileName = file.FileName
    case update.Message.Voice != nil:
        file = update.Message.Voice
        fileName = file.FileName
    }

    if file != nil {
        saveFile(file.FileID, fileName, chatID, module)
    }
}

func saveFile(fileID, fileName string, chatID int64, module string) {
    filePath := filepath.Join(uploadDir, module, fileName)

    path, err := getFilePath(fileID)
    if err != nil {
        sendMessage(chatID, "❌ Error getting file path")
        return
    }

    fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", token, path)
    resp, err := http.Get(fileURL)
    if err != nil {
        sendMessage(chatID, "❌ Error downloading file")
        return
    }
    defer resp.Body.Close()

    os.MkdirAll(filepath.Dir(filePath), 0755)
    out, err := os.Create(filePath)
    if err != nil {
        sendMessage(chatID, "❌ Error saving file")
        return
    }
    defer out.Close()

    _, err = io.Copy(out, resp.Body)
    if err != nil {
        sendMessage(chatID, "❌ Error saving file")
        return
    }

    sendMessage(chatID, fmt.Sprintf("✅ File %s saved successfully!", fileName))
}

func getFilePath(fileID string) (string, error) {
    resp, err := http.Get(apiURL + "/getFile?file_id=" + fileID)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        Ok     bool `json:"ok"`
        Result struct {
            FilePath string `json:"file_path"`
        } `json:"result"`
    }

    err = json.NewDecoder(resp.Body).Decode(&result)
    if err != nil || !result.Ok {
        return "", fmt.Errorf("error getting file path")
    }

    return result.Result.FilePath, nil
}

func sendToAI(text string) string {
    url := "https://openrouter.ai/api/v1/chat/completions"
    payload := map[string]interface{}{
        "model": "openai/gpt-3.5-turbo",
        "messages": []map[string]string{
            {"role": "user", "content": text},
        },
    }

    body, _ := json.Marshal(payload)
    req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+openaiKey)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("HTTP-Referer", "https://your-domain.com")
    req.Header.Set("X-Title", "YourApp")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "❌ Error connecting to AI"
    }
    defer resp.Body.Close()

    var result struct {
        Choices []struct {
            Message struct {
                Content string `json:"content"`
            } `json:"message"`
        } `json:"choices"`
    }
    json.NewDecoder(resp.Body).Decode(&result)

    if len(result.Choices) > 0 {
        return result.Choices[0].Message.Content
    }
    return "❌ No response from AI"
}

func handleCommand(chatID int64, command string, args []string) {
    switch command {
    case "/learn":
        if len(args) < 1 {
            sendMessage(chatID, "❗ لطفاً نام ماژول رو بنویس. مثلا: /learn Mohsen")
            return
        }
        moduleName := args[0]
        userModules[chatID] = moduleName
        moduleDir := filepath.Join(uploadDir, moduleName)
        if _, err := os.Stat(moduleDir); os.IsNotExist(err) {
            os.MkdirAll(moduleDir, 0755)
            sendMessage(chatID, fmt.Sprintf("✅ ماژول %s ایجاد و فعال شد!", moduleName))
        } else {
            sendMessage(chatID, fmt.Sprintf("✅ ماژول %s فعال شد!", moduleName))
        }

    case "/mkdir":
        if len(args) < 1 {
            sendMessage(chatID, "❗ لطفاً نام پوشه رو بنویس. مثلا: /mkdir images")
            return
        }
        folderName := args[0]
        moduleName := userModules[chatID]
        folderPath := filepath.Join(uploadDir, moduleName, folderName)
        err := os.MkdirAll(folderPath, 0755)
        if err != nil {
            sendMessage(chatID, fmt.Sprintf("❌ خطا در ایجاد پوشه: %s", folderName))
            return
        }
        sendMessage(chatID, fmt.Sprintf("✅ پوشه %s ایجاد شد!", folderName))

    case "/touch":
        if len(args) < 1 {
            sendMessage(chatID, "❗ لطفاً نام فایل رو بنویس. مثلا: /touch note.txt")
            return
        }
        fileName := args[0]
        moduleName := userModules[chatID]
        filePath := filepath.Join(uploadDir, moduleName, fileName)
        _, err := os.Create(filePath)
        if err != nil {
            sendMessage(chatID, fmt.Sprintf("❌ خطا در ایجاد فایل: %s", fileName))
            return
        }
        sendMessage(chatID, fmt.Sprintf("✅ فایل %s ایجاد شد!", fileName))

    case "/save":
        if len(args) < 2 {
            sendMessage(chatID, "❗ لطفاً نام فایل و محتوای آن را بنویسید. مثلا: /save note.txt این یک تست است")
            return
        }
        fileName := args[0]
        content := strings.Join(args[1:], " ")
        moduleName := userModules[chatID]
        filePath := filepath.Join(uploadDir, moduleName, fileName)
        err := os.WriteFile(filePath, []byte(content), 0644)
        if err != nil {
            sendMessage(chatID, fmt.Sprintf("❌ خطا در ذخیره فایل: %s", fileName))
            return
        }
        sendMessage(chatID, fmt.Sprintf("✅ فایل %s ذخیره شد!", fileName))

    case "/read":
        if len(args) < 1 {
            sendMessage(chatID, "❗ لطفاً مسیر فایل را بنویسید. مثلا: /read Mohsen/note.txt")
            return
        }
        filePath := filepath.Join(uploadDir, args[0])
        content, err := os.ReadFile(filePath)
        if err != nil {
            sendMessage(chatID, fmt.Sprintf("❌ خطا در خواندن فایل: %s", args[0]))
            return
        }

        if len(content) > 4096 {
            sendDocument(chatID, filepath.Base(args[0]), filePath)
        } else {
            sendMessage(chatID, fmt.Sprintf("\n%s\n", string(content)))
        }

    case "/generate":
        if len(args) < 1 {
            sendMessage(chatID, "❗ لطفاً نام ماژول را بنویسید. مثلا: /generate Mohsen")
            return
        }
        moduleName := args[0]
        sourceDir := filepath.Join(uploadDir, moduleName)

        if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
            sendMessage(chatID, "❌ ماژول پیدا نشد!")
            return
        }

        zipFile := moduleName + ".zip"
        zipPath := filepath.Join(uploadDir, zipFile)

        err := createZipArchive(sourceDir, zipPath)
        if err != nil {
            sendMessage(chatID, "❌ خطا در ساخت فایل ZIP")
            return
        }

        sendDocument(chatID, zipFile, zipPath)
        os.Remove(zipPath)

    case "/chat":
        userStates[chatID] = true
        sendMessage(chatID, "💬 شما در حالت گفتگو با AI هستید. برای خروج از حالت گفتگو /exit را بفرستید.")

    case "/exit":
        userStates[chatID] = false
        sendMessage(chatID, "👋 شما از حالت گفتگو خارج شدید.")

    case "/help":
        helpText := "🛠 راهنمای دستورات:\n" +
            "- /learn <module_name> → فعال‌سازی ماژول\n" +
            "- /mkdir <folder_name> → ایجاد پوشه\n" +
            "- /touch <file_name> → ایجاد فایل خالی\n" +
            "- /save <file_name> <content> → ذخیره متن در فایل\n" +
            "- /read <file_path> → خواندن محتوای فایل\n" +
            "- /generate <module_name> → دریافت فایل ZIP\n" +
            "- /chat → شروع چت با AI\n" +
            "- /exit → خروج از گفتگو با AI\n" +
            "- /help → نمایش این راهنما"

        msg := tgbotapi.NewMessage(chatID, helpText)
        bot.Send(msg)
    }
}

func main() {
    fmt.Println("🚀 Bot started...")

    for {
        updates := getUpdates()
        for _, update := range updates {
            chatID := update.Message.Chat.ID
            if update.Message.Text != "" {
                if strings.HasPrefix(update.Message.Text, "/") {
                    parts := strings.Fields(update.Message.Text)
                    command := parts[0]
                    args := parts[1:]
                    handleCommand(chatID, command, args)
                } else {
                    if userStates[chatID] {
                        response := sendToAI(update.Message.Text)
                        sendMessage(chatID, response)
                    } else {
                        sendMessage(chatID, "⚠️ برای گفتگو با AI از دستور /chat استفاده کنید.")
                    }
                }
            } else {
                handleIncomingFile(update)
            }
        }
        time.Sleep(time.Second)
    }
}

func init() {
    err := godotenv.Load()
    if err != nil {
        fmt.Println("Error loading .env file")
        os.Exit(1)
    }

    token = os.Getenv("BOT_TOKEN")
    openaiKey = os.Getenv("OPENAI_API_KEY")
    uploadDir = os.Getenv("UPLOAD_DIR")
    templateDir = os.Getenv("TEMPLATE_DIR")

    if token == "" || openaiKey == "" || uploadDir == "" || templateDir == "" {
        fmt.Println("Missing required environment variables")
        os.Exit(1)
    }

    os.MkdirAll(uploadDir, 0755)
    os.MkdirAll(templateDir, 0755)

    apiURL = "https://api.telegram.org/bot" + token

    botErr, err := tgbotapi.NewBotAPI(token)
    if err != nil {
        fmt.Println("Error initializing bot:", err)
        os.Exit(1)
    }
    bot = botErr
}

