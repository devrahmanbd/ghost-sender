package notification

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "strings"
    "sync"
    "time"

    "email-campaign-system/pkg/logger"
)

var (
    ErrTelegramNotConfigured = errors.New("telegram not configured")
    ErrInvalidBotToken       = errors.New("invalid bot token")
    ErrInvalidChatID         = errors.New("invalid chat ID")
    ErrMessageTooLong        = errors.New("message exceeds telegram limit")
    ErrTelegramAPIError      = errors.New("telegram API error")
)

const (
    TelegramAPIBaseURL  = "https://api.telegram.org/bot%s/%s"
    MaxMessageLength    = 4096
    MaxCaptionLength    = 1024
    TelegramTimeout     = 30 * time.Second
    TelegramRateLimit   = 30
    TelegramBurstLimit  = 3
)

type ParseMode string

const (
    ParseModeMarkdown   ParseMode = "Markdown"
    ParseModeMarkdownV2 ParseMode = "MarkdownV2"
    ParseModeHTML       ParseMode = "HTML"
)

type TelegramChannel struct {
    mu            sync.RWMutex
    config        *TelegramConfig
    httpClient    *http.Client
    logger        logger.Logger
    stats         *TelegramStats
    rateLimiter   *TelegramRateLimiter
    messageQueue  chan *TelegramMessage
    workers       int
    shutdownCh    chan struct{}
    wg            sync.WaitGroup
}

type TelegramConfig struct {
    BotToken        string
    ChatID          string
    ParseMode       ParseMode
    DisablePreview  bool
    DisableNotification bool
    ThreadID        int
    Timeout         time.Duration
    RetryAttempts   int
    RetryDelay      time.Duration
    Workers         int
    EnableRateLimiting bool
}

type TelegramMessage struct {
    Text                  string
    ChatID                string
    ParseMode             ParseMode
    DisableWebPagePreview bool
    DisableNotification   bool
    ReplyToMessageID      int
    ReplyMarkup           interface{}
    Entities              []MessageEntity
}

type TelegramResponse struct {
    Ok          bool            `json:"ok"`
    Result      json.RawMessage `json:"result,omitempty"`
    ErrorCode   int             `json:"error_code,omitempty"`
    Description string          `json:"description,omitempty"`
}

type MessageEntity struct {
    Type   string `json:"type"`
    Offset int    `json:"offset"`
    Length int    `json:"length"`
    URL    string `json:"url,omitempty"`
}

type InlineKeyboardMarkup struct {
    InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
    Text         string `json:"text"`
    URL          string `json:"url,omitempty"`
    CallbackData string `json:"callback_data,omitempty"`
}

type TelegramStats struct {
    mu              sync.RWMutex
    MessagesSent    int64
    MessagesFailed  int64
    BytesSent       int64
    LastSent        time.Time
    AverageLatency  time.Duration
    TotalLatency    time.Duration
}

type TelegramRateLimiter struct {
    mu           sync.Mutex
    tokens       int
    maxTokens    int
    refillRate   time.Duration
    lastRefill   time.Time
    burstTokens  int
}

func NewTelegramChannel(config *TelegramConfig, log logger.Logger) (*TelegramChannel, error) {
    if config == nil {
        return nil, errors.New("config is nil")
    }

    if err := validateTelegramConfig(config); err != nil {
        return nil, err
    }

    tc := &TelegramChannel{
        config: config,
        httpClient: &http.Client{
            Timeout: config.Timeout,
        },
        logger:       log,
        stats:        &TelegramStats{},
        messageQueue: make(chan *TelegramMessage, 100),
        workers:      config.Workers,
        shutdownCh:   make(chan struct{}),
    }

    if config.EnableRateLimiting {
        tc.rateLimiter = NewTelegramRateLimiter(TelegramRateLimit, TelegramBurstLimit)
    }

    return tc, nil
}

func DefaultTelegramConfig() *TelegramConfig {
    return &TelegramConfig{
        ParseMode:           ParseModeHTML,
        DisablePreview:      false,
        DisableNotification: false,
        Timeout:             TelegramTimeout,
        RetryAttempts:       3,
        RetryDelay:          2 * time.Second,
        Workers:             3,
        EnableRateLimiting:  true,
    }
}

func validateTelegramConfig(config *TelegramConfig) error {
    if config.BotToken == "" {
        return ErrInvalidBotToken
    }

    if config.ChatID == "" {
        return ErrInvalidChatID
    }

    if !strings.HasPrefix(config.BotToken, "bot") && len(config.BotToken) < 40 {
        return ErrInvalidBotToken
    }

    return nil
}

func (tc *TelegramChannel) Start(ctx context.Context) error {
    tc.logger.Info("starting telegram channel workers", logger.Int("workers", tc.workers))

    for i := 0; i < tc.workers; i++ {
        tc.wg.Add(1)
        go tc.worker(ctx, i)
    }

    return nil
}

func (tc *TelegramChannel) Stop() error {
    tc.logger.Info("stopping telegram channel")
    
    close(tc.shutdownCh)
    close(tc.messageQueue)
    tc.wg.Wait()

    return nil
}

func (tc *TelegramChannel) worker(ctx context.Context, id int) {
    defer tc.wg.Done()

    for {
        select {
        case <-ctx.Done():
            return
        case <-tc.shutdownCh:
            return
        case msg, ok := <-tc.messageQueue:
            if !ok {
                return
            }

            if tc.rateLimiter != nil && !tc.rateLimiter.Allow() {
                time.Sleep(tc.rateLimiter.refillRate)
            }

            if err := tc.sendMessageWithRetry(ctx, msg); err != nil {
                tc.logger.Error("failed to send telegram message", logger.Int("worker", id), logger.Error(err))
            }
        }
    }
}

func (tc *TelegramChannel) Send(ctx context.Context, notification *Notification) error {
    if !tc.IsConfigured() {
        return ErrTelegramNotConfigured
    }

    message := tc.formatNotification(notification)

    select {
    case tc.messageQueue <- message:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    case <-time.After(5 * time.Second):
        return errors.New("telegram message queue full")
    }
}

func (tc *TelegramChannel) SendBatch(ctx context.Context, notifications []*Notification) error {
    for _, notification := range notifications {
        if err := tc.Send(ctx, notification); err != nil {
            tc.logger.Error("failed to send notification in batch", logger.Error(err))
        }
    }

    return nil
}

func (tc *TelegramChannel) IsConfigured() bool {
    tc.mu.RLock()
    defer tc.mu.RUnlock()

    return tc.config.BotToken != "" && tc.config.ChatID != ""
}

func (tc *TelegramChannel) TestConnection() error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    return tc.getMe(ctx)
}

func (tc *TelegramChannel) GetStats() ChannelStats {
    tc.stats.mu.RLock()
    defer tc.stats.mu.RUnlock()

    avgDelay := time.Duration(0)
    if tc.stats.MessagesSent > 0 {
        avgDelay = time.Duration(int64(tc.stats.TotalLatency) / tc.stats.MessagesSent)
    }

    return ChannelStats{
        Sent:         tc.stats.MessagesSent,
        Failed:       tc.stats.MessagesFailed,
        LastSent:     tc.stats.LastSent,
        AverageDelay: avgDelay,
    }
}

func (tc *TelegramChannel) sendMessageWithRetry(ctx context.Context, message *TelegramMessage) error {
    var lastErr error

    for attempt := 0; attempt <= tc.config.RetryAttempts; attempt++ {
        if attempt > 0 {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(tc.config.RetryDelay * time.Duration(attempt)):
            }
        }

        err := tc.sendMessage(ctx, message)
        if err == nil {
            return nil
        }

        lastErr = err
        tc.logger.Warn("telegram send failed, retrying", logger.Int("attempt", attempt+1), logger.Error(err))
    }

    tc.recordFailure()
    return fmt.Errorf("failed after %d attempts: %w", tc.config.RetryAttempts+1, lastErr)
}

func (tc *TelegramChannel) sendMessage(ctx context.Context, message *TelegramMessage) error {
    startTime := time.Now()

    if len(message.Text) > MaxMessageLength {
        return tc.sendLongMessage(ctx, message)
    }

    payload := map[string]interface{}{
        "chat_id":    message.ChatID,
        "text":       message.Text,
        "parse_mode": string(message.ParseMode),
    }

    if message.DisableWebPagePreview {
        payload["disable_web_page_preview"] = true
    }

    if message.DisableNotification {
        payload["disable_notification"] = true
    }

    if message.ReplyToMessageID > 0 {
        payload["reply_to_message_id"] = message.ReplyToMessageID
    }

    if message.ReplyMarkup != nil {
        payload["reply_markup"] = message.ReplyMarkup
    }

    resp, err := tc.makeAPIRequest(ctx, "sendMessage", payload)
    if err != nil {
        return err
    }

    if !resp.Ok {
        return fmt.Errorf("%w: %s (code: %d)", ErrTelegramAPIError, resp.Description, resp.ErrorCode)
    }

    tc.recordSuccess(time.Since(startTime), len(message.Text))
    return nil
}

func (tc *TelegramChannel) sendLongMessage(ctx context.Context, message *TelegramMessage) error {
    text := message.Text
    chunks := splitMessage(text, MaxMessageLength-100)

    for i, chunk := range chunks {
        chunkMsg := &TelegramMessage{
            Text:                  chunk,
            ChatID:                message.ChatID,
            ParseMode:             message.ParseMode,
            DisableWebPagePreview: message.DisableWebPagePreview,
            DisableNotification:   message.DisableNotification,
        }

        if i == len(chunks)-1 {
            chunkMsg.ReplyMarkup = message.ReplyMarkup
        }

        if err := tc.sendMessage(ctx, chunkMsg); err != nil {
            return err
        }

        if i < len(chunks)-1 {
            time.Sleep(time.Second)
        }
    }

    return nil
}

func (tc *TelegramChannel) SendPhoto(ctx context.Context, chatID, photoURL, caption string) error {
    payload := map[string]interface{}{
        "chat_id": chatID,
        "photo":   photoURL,
    }

    if caption != "" {
        if len(caption) > MaxCaptionLength {
            caption = caption[:MaxCaptionLength-3] + "..."
        }
        payload["caption"] = caption
        payload["parse_mode"] = string(tc.config.ParseMode)
    }

    resp, err := tc.makeAPIRequest(ctx, "sendPhoto", payload)
    if err != nil {
        return err
    }

    if !resp.Ok {
        return fmt.Errorf("%w: %s", ErrTelegramAPIError, resp.Description)
    }

    return nil
}

func (tc *TelegramChannel) SendDocument(ctx context.Context, chatID, documentURL, caption string) error {
    payload := map[string]interface{}{
        "chat_id":  chatID,
        "document": documentURL,
    }

    if caption != "" {
        if len(caption) > MaxCaptionLength {
            caption = caption[:MaxCaptionLength-3] + "..."
        }
        payload["caption"] = caption
    }

    resp, err := tc.makeAPIRequest(ctx, "sendDocument", payload)
    if err != nil {
        return err
    }

    if !resp.Ok {
        return fmt.Errorf("%w: %s", ErrTelegramAPIError, resp.Description)
    }

    return nil
}

func (tc *TelegramChannel) getMe(ctx context.Context) error {
    resp, err := tc.makeAPIRequest(ctx, "getMe", nil)
    if err != nil {
        return err
    }

    if !resp.Ok {
        return fmt.Errorf("%w: %s", ErrTelegramAPIError, resp.Description)
    }

    return nil
}

func (tc *TelegramChannel) makeAPIRequest(ctx context.Context, method string, payload map[string]interface{}) (*TelegramResponse, error) {
    url := fmt.Sprintf(TelegramAPIBaseURL, tc.config.BotToken, method)

    var body []byte
    var err error

    if payload != nil {
        body, err = json.Marshal(payload)
        if err != nil {
            return nil, fmt.Errorf("failed to marshal payload: %w", err)
        }
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")

    resp, err := tc.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to send request: %w", err)
    }
    defer resp.Body.Close()

    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }

    var telegramResp TelegramResponse
    if err := json.Unmarshal(respBody, &telegramResp); err != nil {
        return nil, fmt.Errorf("failed to unmarshal response: %w", err)
    }

    return &telegramResp, nil
}

func (tc *TelegramChannel) formatNotification(notification *Notification) *TelegramMessage {
    text := tc.buildMessageText(notification)

    message := &TelegramMessage{
        Text:                  text,
        ChatID:                tc.config.ChatID,
        ParseMode:             tc.config.ParseMode,
        DisableWebPagePreview: tc.config.DisablePreview,
        DisableNotification:   tc.config.DisableNotification,
    }

    if notification.Level == LevelError || notification.Level == LevelCritical {
        message.DisableNotification = false
    }

    return message
}

func (tc *TelegramChannel) buildMessageText(notification *Notification) string {
    emoji := tc.getEmojiForLevel(notification.Level)
    
    var sb strings.Builder
    
    sb.WriteString(fmt.Sprintf("%s <b>%s</b>\n\n", emoji, notification.Title))
    sb.WriteString(notification.Message)
    
    if len(notification.Data) > 0 {
        sb.WriteString("\n\n<b>Details:</b>\n")
        for key, value := range notification.Data {
            sb.WriteString(fmt.Sprintf("• %s: %v\n", key, value))
        }
    }
    
    sb.WriteString(fmt.Sprintf("\n<i>%s</i>", notification.Timestamp.Format("2006-01-02 15:04:05")))
    
    return sb.String()
}

func (tc *TelegramChannel) getEmojiForLevel(level NotificationLevel) string {
    switch level {
    case LevelSuccess:
        return "✅"
    case LevelInfo:
        return "ℹ️"
    case LevelWarning:
        return "⚠️"
    case LevelError:
        return "❌"
    case LevelCritical:
        return "🚨"
    default:
        return "📢"
    }
}

func (tc *TelegramChannel) recordSuccess(latency time.Duration, bytes int) {
    tc.stats.mu.Lock()
    defer tc.stats.mu.Unlock()

    tc.stats.MessagesSent++
    tc.stats.BytesSent += int64(bytes)
    tc.stats.LastSent = time.Now()
    tc.stats.TotalLatency += latency
    tc.stats.AverageLatency = time.Duration(int64(tc.stats.TotalLatency) / tc.stats.MessagesSent)
}

func (tc *TelegramChannel) recordFailure() {
    tc.stats.mu.Lock()
    defer tc.stats.mu.Unlock()

    tc.stats.MessagesFailed++
}

func (tc *TelegramChannel) UpdateConfig(config *TelegramConfig) error {
    if err := validateTelegramConfig(config); err != nil {
        return err
    }

    tc.mu.Lock()
    defer tc.mu.Unlock()

    tc.config = config
    tc.logger.Info("telegram config updated")

    return nil
}

func (tc *TelegramChannel) GetBotInfo(ctx context.Context) (map[string]interface{}, error) {
    resp, err := tc.makeAPIRequest(ctx, "getMe", nil)
    if err != nil {
        return nil, err
    }

    if !resp.Ok {
        return nil, fmt.Errorf("%w: %s", ErrTelegramAPIError, resp.Description)
    }

    var botInfo map[string]interface{}
    if err := json.Unmarshal(resp.Result, &botInfo); err != nil {
        return nil, err
    }

    return botInfo, nil
}

func NewTelegramRateLimiter(tokensPerMinute, burstTokens int) *TelegramRateLimiter {
    return &TelegramRateLimiter{
        tokens:      tokensPerMinute,
        maxTokens:   tokensPerMinute,
        refillRate:  time.Minute / time.Duration(tokensPerMinute),
        lastRefill:  time.Now(),
        burstTokens: burstTokens,
    }
}

func (trl *TelegramRateLimiter) Allow() bool {
    trl.mu.Lock()
    defer trl.mu.Unlock()

    now := time.Now()
    elapsed := now.Sub(trl.lastRefill)

    tokensToAdd := int(elapsed / trl.refillRate)
    if tokensToAdd > 0 {
        trl.tokens = min(trl.tokens+tokensToAdd, trl.maxTokens)
        trl.lastRefill = now
    }

    if trl.tokens > 0 {
        trl.tokens--
        return true
    }

    return false
}

func (trl *TelegramRateLimiter) WaitTime() time.Duration {
    trl.mu.Lock()
    defer trl.mu.Unlock()

    if trl.tokens > 0 {
        return 0
    }

    return trl.refillRate
}

func splitMessage(text string, maxLength int) []string {
    if len(text) <= maxLength {
        return []string{text}
    }

    var chunks []string
    runes := []rune(text)

    for len(runes) > 0 {
        end := min(len(runes), maxLength)
        
        if end < len(runes) {
            for i := end - 1; i > end-100 && i > 0; i-- {
                if runes[i] == '\n' || runes[i] == ' ' {
                    end = i
                    break
                }
            }
        }

        chunks = append(chunks, string(runes[:end]))
        runes = runes[end:]
    }

    return chunks
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

func EscapeMarkdownV2(text string) string {
    specialChars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
    
    for _, char := range specialChars {
        text = strings.ReplaceAll(text, char, "\\"+char)
    }
    
    return text
}

func EscapeHTML(text string) string {
    text = strings.ReplaceAll(text, "&", "&amp;")
    text = strings.ReplaceAll(text, "<", "&lt;")
    text = strings.ReplaceAll(text, ">", "&gt;")
    return text
}

func CreateInlineKeyboard(buttons [][]InlineKeyboardButton) *InlineKeyboardMarkup {
    return &InlineKeyboardMarkup{
        InlineKeyboard: buttons,
    }
}

func CreateButton(text, url string) InlineKeyboardButton {
    return InlineKeyboardButton{
        Text: text,
        URL:  url,
    }
}
