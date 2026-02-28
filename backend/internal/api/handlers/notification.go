package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"email-campaign-system/internal/api/websocket"
	"email-campaign-system/internal/core/notification"
	"email-campaign-system/internal/storage/repository"
	"email-campaign-system/pkg/errors"
	"email-campaign-system/pkg/logger"
	"email-campaign-system/pkg/validator"
)

// ============================================================================
// HANDLER STRUCT
// ============================================================================

type NotificationHandler struct {
	configRepo      *repository.ConfigRepository
	validator       *validator.Validator
	wsHub           *websocket.Hub
	logger          logger.Logger
	notificationMgr *notification.NotificationManager
	telegramChannel *notification.TelegramChannel
}

func NewNotificationHandler(
	configRepo *repository.ConfigRepository,
	validator *validator.Validator,
	wsHub *websocket.Hub,
	log logger.Logger,
	notificationMgr *notification.NotificationManager,
) *NotificationHandler {
	return &NotificationHandler{
		configRepo:      configRepo,
		validator:       validator,
		wsHub:           wsHub,
		logger:          log,
		notificationMgr: notificationMgr,
	}
}

// ============================================================================
// REQUEST/RESPONSE TYPES
// ============================================================================

type NotificationConfigResponse struct {
	Enabled            bool              `json:"enabled"`
	TelegramEnabled    bool              `json:"telegram_enabled"`
	TelegramConfigured bool              `json:"telegram_configured"`
	BotUsername        string            `json:"bot_username"`
	ChatID             string            `json:"chat_id"`
	Preferences        NotificationPrefs `json:"preferences"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

type NotificationPrefs struct {
	NotifyOnStart          bool `json:"notify_on_start"`
	NotifyOnComplete       bool `json:"notify_on_complete"`
	NotifyOnError          bool `json:"notify_on_error"`
	NotifyOnPause          bool `json:"notify_on_pause"`
	NotifyOnResume         bool `json:"notify_on_resume"`
	NotifyOnCancel         bool `json:"notify_on_cancel"`
	NotifyOnAccountSuspend bool `json:"notify_on_account_suspend"`
}

type UpdateTelegramConfigRequest struct {
	BotToken string `json:"bot_token" validate:"required"`
	ChatID   string `json:"chat_id" validate:"required"`
}

type UpdatePreferencesRequest struct {
	NotifyOnStart          bool `json:"notify_on_start"`
	NotifyOnComplete       bool `json:"notify_on_complete"`
	NotifyOnError          bool `json:"notify_on_error"`
	NotifyOnPause          bool `json:"notify_on_pause"`
	NotifyOnResume         bool `json:"notify_on_resume"`
	NotifyOnCancel         bool `json:"notify_on_cancel"`
	NotifyOnAccountSuspend bool `json:"notify_on_account_suspend"`
}

type TestNotificationRequest struct {
	Message string `json:"message"`
}

type NotificationHistoryResponse struct {
	ID        uint      `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	SentAt    time.Time `json:"sent_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ============================================================================
// HANDLERS
// ============================================================================

// GetConfig returns the current notification configuration
func (h *NotificationHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get configuration from repository
	entries, err := h.configRepo.GetSection(ctx, "notification")
	if err != nil {
		h.logger.Error("Failed to get notification config", logger.Error(err))
	}

	// Parse configuration
	enabled := h.notificationMgr.IsEnabled()
	telegramEnabled := false
	botUsername := ""
	chatID := ""
	prefs := NotificationPrefs{
		NotifyOnStart:          true,
		NotifyOnComplete:       true,
		NotifyOnError:          true,
		NotifyOnPause:          false,
		NotifyOnResume:         false,
		NotifyOnCancel:         true,
		NotifyOnAccountSuspend: true,
	}

	for _, entry := range entries {
		if entry.Key == "enabled" {
			enabled = entry.Value == "true"
		} else if entry.Key == "telegram" && entry.Metadata != nil {
			// Type assert Metadata to map[string]interface{}
			if metadata, ok := entry.Metadata.(map[string]interface{}); ok {
				if token, ok := metadata["bot_token"].(string); ok && token != "" {
					telegramEnabled = true
				}
				if username, ok := metadata["bot_username"].(string); ok {
					botUsername = username
				}
				if id, ok := metadata["chat_id"].(string); ok {
					chatID = id
				}
			}
		} else if entry.Key == "preferences" && entry.Metadata != nil {
			if metadata, ok := entry.Metadata.(map[string]interface{}); ok {
				if v, ok := metadata["notify_on_start"].(bool); ok {
					prefs.NotifyOnStart = v
				}
				if v, ok := metadata["notify_on_complete"].(bool); ok {
					prefs.NotifyOnComplete = v
				}
				if v, ok := metadata["notify_on_error"].(bool); ok {
					prefs.NotifyOnError = v
				}
				if v, ok := metadata["notify_on_pause"].(bool); ok {
					prefs.NotifyOnPause = v
				}
				if v, ok := metadata["notify_on_resume"].(bool); ok {
					prefs.NotifyOnResume = v
				}
				if v, ok := metadata["notify_on_cancel"].(bool); ok {
					prefs.NotifyOnCancel = v
				}
				if v, ok := metadata["notify_on_account_suspend"].(bool); ok {
					prefs.NotifyOnAccountSuspend = v
				}
			}
		}
	}

	response := NotificationConfigResponse{
		Enabled:            enabled,
		TelegramEnabled:    telegramEnabled,
		TelegramConfigured: chatID != "",
		BotUsername:        botUsername,
		ChatID:             chatID,
		Preferences:        prefs,
		UpdatedAt:          time.Now(),
	}

	h.respondJSON(w, http.StatusOK, response)
}

// UpdateTelegramConfig updates the Telegram bot configuration
func (h *NotificationHandler) UpdateTelegramConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req UpdateTelegramConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("Validation failed", []string{err.Error()}))
		return
	}

	// Create and configure Telegram channel
	telegramConfig := &notification.TelegramConfig{
		BotToken:            req.BotToken,
		ChatID:              req.ChatID,
		ParseMode:           notification.ParseModeHTML,
		DisablePreview:      false,
		DisableNotification: false,
		Timeout:             30 * time.Second,
		RetryAttempts:       3,
		RetryDelay:          2 * time.Second,
		Workers:             3,
		EnableRateLimiting:  true,
	}

	// Create new Telegram channel
	telegramChannel, err := notification.NewTelegramChannel(telegramConfig, h.logger)
	if err != nil {
		h.logger.Error("Failed to create Telegram channel", logger.Error(err))
		h.respondError(w, errors.Internal("Failed to configure Telegram"))
		return
	}

	// Test connection
	if err := telegramChannel.TestConnection(); err != nil {
		h.logger.Error("Failed to test Telegram connection", logger.Error(err))
		h.respondError(w, errors.BadRequest("Failed to connect to Telegram. Please check your bot token and chat ID"))
		return
	}

	// Get bot info
	botInfo, err := telegramChannel.GetBotInfo(ctx)
	if err != nil {
		h.logger.Warn("Failed to get bot info", logger.Error(err))
	}

	botUsername := ""
	if botInfo != nil {
		if username, ok := botInfo["username"].(string); ok {
			botUsername = username
		}
	}

	// Start the Telegram channel
	if err := telegramChannel.Start(ctx); err != nil {
		h.logger.Error("Failed to start Telegram channel", logger.Error(err))
		h.respondError(w, errors.Internal("Failed to start Telegram channel"))
		return
	}

	// Register channel with notification manager
	if err := h.notificationMgr.RegisterChannel(notification.ChannelTelegram, telegramChannel); err != nil {
		h.logger.Error("Failed to register Telegram channel", logger.Error(err))
		_ = telegramChannel.Stop()
		h.respondError(w, errors.Internal("Failed to register Telegram channel"))
		return
	}

	// Store the channel reference
	h.telegramChannel = telegramChannel

	// Save configuration using Set method
	configEntry := &repository.ConfigEntry{
		Section: "notification",
		Key:     "telegram",
		Value:   req.ChatID,
		Type:    "json",
		Metadata: map[string]interface{}{
			"bot_token":    req.BotToken,
			"chat_id":      req.ChatID,
			"bot_username": botUsername,
			"enabled":      true,
		},
		UpdatedBy: "system",
	}

	if err := h.configRepo.Set(ctx, configEntry); err != nil {
		h.logger.Error("Failed to persist Telegram config", logger.Error(err))
		h.respondError(w, errors.Internal("Failed to save configuration"))
		return
	}

	h.logger.Info("Telegram configuration updated successfully")

	// Broadcast WebSocket message
	h.wsHub.Broadcast(&websocket.Message{
		Type: "notification_config_updated",
		Data: json.RawMessage(`{"telegram_configured": true}`),
	})

	response := NotificationConfigResponse{
		Enabled:            true,
		TelegramEnabled:    true,
		TelegramConfigured: true,
		BotUsername:        botUsername,
		ChatID:             req.ChatID,
		Preferences: NotificationPrefs{
			NotifyOnStart:    true,
			NotifyOnComplete: true,
			NotifyOnError:    true,
		},
		UpdatedAt: time.Now(),
	}

	h.respondJSON(w, http.StatusOK, response)
}

// TestNotification sends a test notification
func (h *NotificationHandler) TestNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req TestNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Message = "🎉 This is a test notification from Email Campaign System"
	}

	if req.Message == "" {
		req.Message = "🎉 This is a test notification from Email Campaign System"
	}

	h.logger.Info("Test notification requested", logger.String("message", req.Message))

	// Check if notifications are enabled
	if !h.notificationMgr.IsEnabled() {
		h.respondError(w, errors.BadRequest("Notifications are disabled"))
		return
	}

	// Create test notification
	testNotification := &notification.Notification{
		Channel:   notification.ChannelTelegram,
		Level:     notification.LevelInfo,
		Event:     "test.notification",
		Title:     "🧪 Test Notification",
		Message:   req.Message,
		Data: map[string]interface{}{
			"test":      true,
			"timestamp": time.Now().Format(time.RFC3339),
			"source":    "api_handler",
		},
		Timestamp: time.Now(),
		Priority:  5,
	}

	// Send notification through notification manager
	if err := h.notificationMgr.Send(ctx, testNotification); err != nil {
		h.logger.Error("Failed to send test notification", logger.Error(err))
		h.respondError(w, errors.Internal("Failed to send test notification: "+err.Error()))
		return
	}

	h.logger.Info("Test notification sent successfully")

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Test notification sent successfully",
		"details": map[string]interface{}{
			"channel": "telegram",
			"sent_at": time.Now().Format(time.RFC3339),
		},
	})
}

// EnableNotifications enables the notification system
func (h *NotificationHandler) EnableNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Enable notification manager
	h.notificationMgr.Enable()

	// Save configuration
	configEntry := &repository.ConfigEntry{
		Section:   "notification",
		Key:       "enabled",
		Value:     "true",
		Type:      "boolean",
		UpdatedBy: "system",
	}

	if err := h.configRepo.Set(ctx, configEntry); err != nil {
		h.logger.Error("Failed to persist notification enable state", logger.Error(err))
		h.respondError(w, errors.Internal("Failed to enable notifications"))
		return
	}

	h.logger.Info("Notifications enabled")

	// Broadcast WebSocket message
	h.wsHub.Broadcast(&websocket.Message{
		Type: "notification_enabled",
		Data: json.RawMessage(`{"enabled": true}`),
	})

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Notifications enabled successfully",
		"enabled": true,
	})
}

// DisableNotifications disables the notification system
func (h *NotificationHandler) DisableNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Disable notification manager
	h.notificationMgr.Disable()

	// Save configuration
	configEntry := &repository.ConfigEntry{
		Section:   "notification",
		Key:       "enabled",
		Value:     "false",
		Type:      "boolean",
		UpdatedBy: "system",
	}

	if err := h.configRepo.Set(ctx, configEntry); err != nil {
		h.logger.Error("Failed to persist notification disable state", logger.Error(err))
		h.respondError(w, errors.Internal("Failed to disable notifications"))
		return
	}

	h.logger.Info("Notifications disabled")

	// Broadcast WebSocket message
	h.wsHub.Broadcast(&websocket.Message{
		Type: "notification_disabled",
		Data: json.RawMessage(`{"enabled": false}`),
	})

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Notifications disabled successfully",
		"enabled": false,
	})
}

// UpdatePreferences updates notification preferences
func (h *NotificationHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	// Save configuration
	configEntry := &repository.ConfigEntry{
		Section: "notification",
		Key:     "preferences",
		Value:   "",
		Type:    "json",
		Metadata: map[string]interface{}{
			"notify_on_start":           req.NotifyOnStart,
			"notify_on_complete":        req.NotifyOnComplete,
			"notify_on_error":           req.NotifyOnError,
			"notify_on_pause":           req.NotifyOnPause,
			"notify_on_resume":          req.NotifyOnResume,
			"notify_on_cancel":          req.NotifyOnCancel,
			"notify_on_account_suspend": req.NotifyOnAccountSuspend,
		},
		UpdatedBy: "system",
	}

	if err := h.configRepo.Set(ctx, configEntry); err != nil {
		h.logger.Error("Failed to persist notification preferences", logger.Error(err))
		h.respondError(w, errors.Internal("Failed to save preferences"))
		return
	}

	// Configure event filters based on preferences
	h.notificationMgr.FilterEvent(notification.EventCampaignStarted, !req.NotifyOnStart)
	h.notificationMgr.FilterEvent(notification.EventCampaignCompleted, !req.NotifyOnComplete)
	h.notificationMgr.FilterEvent(notification.EventCampaignFailed, !req.NotifyOnError)
	h.notificationMgr.FilterEvent(notification.EventCampaignPaused, !req.NotifyOnPause)
	h.notificationMgr.FilterEvent(notification.EventCampaignResumed, !req.NotifyOnResume)
	h.notificationMgr.FilterEvent(notification.EventAccountSuspended, !req.NotifyOnAccountSuspend)

	h.logger.Info("Notification preferences updated successfully")

	// Broadcast WebSocket message with JSON
	prefsJSON, _ := json.Marshal(req)
	h.wsHub.Broadcast(&websocket.Message{
		Type: "notification_preferences_updated",
		Data: json.RawMessage(prefsJSON),
	})

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"message":     "Notification preferences updated successfully",
		"preferences": req,
	})
}

// GetHistory returns notification history
func (h *NotificationHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Notification history requested")

	// Get metrics from notification manager
	metrics := h.notificationMgr.GetMetrics()

	// For now, return metrics as history
	response := []NotificationHistoryResponse{}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"history": response,
		"total":   0,
		"metrics": map[string]interface{}{
			"total_sent":   metrics.TotalSent,
			"total_failed": metrics.TotalFailed,
			"by_channel":   metrics.ByChannel,
			"by_level":     metrics.ByLevel,
			"by_event":     metrics.ByEvent,
		},
	})
}

// GetStats returns notification statistics
func (h *NotificationHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	metrics := h.notificationMgr.GetMetrics()

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"stats": map[string]interface{}{
			"total_sent":        metrics.TotalSent,
			"total_failed":      metrics.TotalFailed,
			"by_channel":        metrics.ByChannel,
			"by_level":          metrics.ByLevel,
			"by_event":          metrics.ByEvent,
			"average_latency":   metrics.AverageLatency.String(),
			"last_notification": metrics.LastNotification.Format(time.RFC3339),
		},
	})
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func (h *NotificationHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", logger.Error(err))
	}
}

func (h *NotificationHandler) respondError(w http.ResponseWriter, err error) {
	var status int
	var message string

	// Check if it's our custom error type
	if appErr, ok := err.(*errors.Error); ok {
		status = appErr.StatusCode
		message = appErr.Message
	} else {
		status = http.StatusInternalServerError
		message = "Internal server error"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := map[string]interface{}{
		"error":   message,
		"status":  status,
		"success": false,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode error response", logger.Error(err))
	}
}
// GetBotInfo returns Telegram bot information
func (h *NotificationHandler) GetBotInfo(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Check if Telegram channel is configured
    if h.telegramChannel == nil {
        h.respondError(w, errors.BadRequest("Telegram not configured"))
        return
    }

    // Get bot information from Telegram
    botInfo, err := h.telegramChannel.GetBotInfo(ctx)
    if err != nil {
        h.logger.Error("Failed to get Telegram bot info", logger.Error(err))
        h.respondError(w, errors.Internal("Failed to retrieve bot information"))
        return
    }

    // Extract bot details
    botUsername := ""
    botName := ""
    botID := int64(0)
    isActive := false

    if botInfo != nil {
        if username, ok := botInfo["username"].(string); ok {
            botUsername = username
        }
        if firstName, ok := botInfo["first_name"].(string); ok {
            botName = firstName
        }
        if id, ok := botInfo["id"].(float64); ok {
            botID = int64(id)
        }
        if canJoinGroups, ok := botInfo["can_join_groups"].(bool); ok {
            isActive = canJoinGroups
        }
    }

    // Get chat ID from configuration
    entries, err := h.configRepo.GetSection(ctx, "notification")
    chatID := ""
    if err == nil {
        for _, entry := range entries {
            if entry.Key == "telegram" && entry.Metadata != nil {
                if metadata, ok := entry.Metadata.(map[string]interface{}); ok {
                    if id, ok := metadata["chat_id"].(string); ok {
                        chatID = id
                    }
                }
            }
        }
    }

    response := map[string]interface{}{
        "bot_id":       botID,
        "bot_name":     botName,
        "bot_username": botUsername,
        "is_connected": true,
        "is_active":    isActive,
        "chat_id":      chatID,
        "status":       "active",
        "full_info":    botInfo,
    }

    h.logger.Info("Bot info retrieved successfully", 
        logger.String("bot_username", botUsername))

    h.respondJSON(w, http.StatusOK, response)
}
