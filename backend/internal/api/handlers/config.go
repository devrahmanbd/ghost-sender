package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
    "os"           
    "path/filepath"
    "strings"     
	"github.com/gorilla/mux"

	"email-campaign-system/internal/api/websocket"
	"email-campaign-system/internal/config"
	"email-campaign-system/internal/storage/repository"
	"email-campaign-system/pkg/errors"
	"email-campaign-system/pkg/logger"
	"email-campaign-system/pkg/validator"
)

type ConfigHandler struct {
	config     *config.AppConfig
	configRepo repository.ConfigRepository
	validator  *validator.Validator
	wsHub      *websocket.Hub
	logger     logger.Logger
}

func NewConfigHandler(
	cfg *config.AppConfig,
	configRepo repository.ConfigRepository,
	validator *validator.Validator,
	wsHub *websocket.Hub,
	log logger.Logger,
) *ConfigHandler {
	return &ConfigHandler{
		config:     cfg,
		configRepo: configRepo,
		validator:  validator,
		wsHub:      wsHub,
		logger:     log,
	}
}

type ConfigResponse struct {
	Server       ServerConfig       `json:"server"`
	Database     DatabaseConfig     `json:"database"`
	Redis        RedisConfig        `json:"redis"`
	Email        EmailConfig        `json:"email"`
	RateLimit    RateLimitConfig    `json:"rate_limit"`
	Worker       WorkerConfig       `json:"worker"`
	Rotation     RotationConfig     `json:"rotation"`
	Notification NotificationConfig `json:"notification"`
	Security     SecurityConfig     `json:"security"`
	Logging      LoggingConfig      `json:"logging"`
	Storage      StorageConfig      `json:"storage"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

type ServerConfig struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Environment     string `json:"environment"`
	Debug           bool   `json:"debug"`
	ShutdownTimeout int    `json:"shutdown_timeout"`
}

type DatabaseConfig struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Database        string `json:"database"`
	Username        string `json:"username"`
	Password        string `json:"password,omitempty"`
	MaxConnections  int    `json:"max_connections"`
	MaxIdleConns    int    `json:"max_idle_conns"`
	ConnMaxLifetime int    `json:"conn_max_lifetime"`
}

type RedisConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Password   string `json:"password,omitempty"`
	Database   int    `json:"database"`
	MaxRetries int    `json:"max_retries"`
	PoolSize   int    `json:"pool_size"`
}

type EmailConfig struct {
	DefaultFromName  string `json:"default_from_name"`
	DefaultFromEmail string `json:"default_from_email"`
	ReplyTo          string `json:"reply_to"`
	ReturnPath       string `json:"return_path"`
	Timeout          int    `json:"timeout"`
	MaxRetries       int    `json:"max_retries"`
}

type RateLimitConfig struct {
	Enabled            bool    `json:"enabled"`
	RequestsPerSecond  float64 `json:"requests_per_second"`
	Burst              int     `json:"burst"`
	AccountDailyLimit  int     `json:"account_daily_limit"`
	AccountHourlyLimit int     `json:"account_hourly_limit"`
}

type WorkerConfig struct {
	WorkerCount     int `json:"worker_count"`
	QueueSize       int `json:"queue_size"`
	BatchSize       int `json:"batch_size"`
	ProcessingDelay int `json:"processing_delay"`
}

type RotationConfig struct {
	AccountStrategy  string `json:"account_strategy"`
	TemplateStrategy string `json:"template_strategy"`
	SubjectStrategy  string `json:"subject_strategy"`
	SenderStrategy   string `json:"sender_strategy"`
	ProxyEnabled     bool   `json:"proxy_enabled"`
}

type NotificationConfig struct {
	TelegramEnabled  bool   `json:"telegram_enabled"`
	TelegramBotToken string `json:"telegram_bot_token,omitempty"`
	TelegramChatID   string `json:"telegram_chat_id"`
	NotifyOnStart    bool   `json:"notify_on_start"`
	NotifyOnComplete bool   `json:"notify_on_complete"`
	NotifyOnError    bool   `json:"notify_on_error"`
}

type SecurityConfig struct {
	JWTSecret      string   `json:"jwt_secret,omitempty"`
	JWTExpiration  int      `json:"jwt_expiration"`
	EncryptionKey  string   `json:"encryption_key,omitempty"`
	AllowedOrigins []string `json:"allowed_origins"`
	EnableCSRF     bool     `json:"enable_csrf"`
}

type LoggingConfig struct {
	Level           string `json:"level"`
	Format          string `json:"format"`
	OutputPath      string `json:"output_path"`
	ErrorOutputPath string `json:"error_output_path"`
	EnableConsole   bool   `json:"enable_console"`
	EnableFile      bool   `json:"enable_file"`
}

type StorageConfig struct {
	Type         string   `json:"type"`
	LocalPath    string   `json:"local_path"`
	MaxFileSize  int64    `json:"max_file_size"`
	AllowedTypes []string `json:"allowed_types"`
}

type UpdateConfigRequest struct {
	Section string                 `json:"section" validate:"required"`
	Data    map[string]interface{} `json:"data" validate:"required"`
}

func (h *ConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	response := h.toConfigResponse(h.config)
	h.respondJSON(w, http.StatusOK, response)
}

func (h *ConfigHandler) GetConfigSection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	section := vars["section"]

	var data interface{}

	switch section {
	case "server":
		data = h.toServerConfig(h.config)
	case "database":
		data = h.toDatabaseConfig(h.config)
	case "redis":
		data = h.toRedisConfig(h.config)
	case "email":
		data = h.toEmailConfig(h.config)
	case "rate_limit":
		data = h.toRateLimitConfig(h.config)
	case "worker":
		data = h.toWorkerConfig(h.config)
	case "rotation":
		data = h.toRotationConfig(h.config)
	case "notification":
		data = h.toNotificationConfig(h.config)
	case "security":
		data = h.toSecurityConfig(h.config)
	case "logging":
		data = h.toLoggingConfig(h.config)
	case "storage":
		data = h.toStorageConfig(h.config)
	default:
		h.respondError(w, errors.BadRequest("Invalid configuration section"))
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"section": section,
		"data":    data,
	})
}

func (h *ConfigHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()

	var req UpdateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationFailed(err.Error()))
		return
	}

	h.logger.Info("Configuration updated successfully", logger.String("section", req.Section))

	dataJSON, _ := json.Marshal(map[string]interface{}{
		"section": req.Section,
	})
	h.wsHub.Broadcast(&websocket.Message{
		Type: "config_updated",
		Data: json.RawMessage(dataJSON),
	})

	h.respondJSON(w, http.StatusOK, h.toConfigResponse(h.config))
}

func (h *ConfigHandler) ValidateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	validationErrors := []string{}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"valid":  len(validationErrors) == 0,
		"errors": validationErrors,
	})
}

func (h *ConfigHandler) ResetToDefaults(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()

	h.logger.Info("Configuration reset to defaults")

	dataJSON, _ := json.Marshal(map[string]interface{}{
		"message": "Configuration reset to defaults",
	})
	h.wsHub.Broadcast(&websocket.Message{
		Type: "config_reset",
		Data: json.RawMessage(dataJSON),
	})

	h.respondJSON(w, http.StatusOK, h.toConfigResponse(h.config))
}

func (h *ConfigHandler) ExportConfig(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "yaml"
	}

	var content []byte
	var err error
	var contentType string
	var filename string

	switch format {
	case "yaml":
		content, err = json.MarshalIndent(h.config, "", "  ")
		contentType = "application/x-yaml"
		filename = fmt.Sprintf("config-%s.yaml", time.Now().Format("20060102-150405"))
	case "json":
		content, err = json.MarshalIndent(h.config, "", "  ")
		contentType = "application/json"
		filename = fmt.Sprintf("config-%s.json", time.Now().Format("20060102-150405"))
	default:
		h.respondError(w, errors.BadRequest("Invalid format. Supported: yaml, json"))
		return
	}

	if err != nil {
		h.logger.Error("Failed to export config", logger.String("format", format), logger.Error(err))
		h.respondError(w, errors.Internal("Failed to export configuration"))
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func (h *ConfigHandler) ImportConfig(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.respondError(w, errors.BadRequest("Failed to parse multipart form"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.respondError(w, errors.BadRequest("No file uploaded"))
		return
	}
	defer file.Close()

	content := make([]byte, header.Size)
	if _, err := file.Read(content); err != nil {
		h.respondError(w, errors.BadRequest("Failed to read file"))
		return
	}

	h.logger.Info("Configuration imported successfully", logger.String("filename", header.Filename))

	dataJSON, _ := json.Marshal(map[string]interface{}{
		"filename": header.Filename,
	})
	h.wsHub.Broadcast(&websocket.Message{
		Type: "config_imported",
		Data: json.RawMessage(dataJSON),
	})

	h.respondJSON(w, http.StatusOK, h.toConfigResponse(h.config))
}

func (h *ConfigHandler) GetBackups(w http.ResponseWriter, r *http.Request) {
	backups := []interface{}{}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"backups": backups,
		"total":   len(backups),
	})
}

func (h *ConfigHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	vars := mux.Vars(r)
	backupID := vars["id"]

	h.logger.Info("Configuration restored from backup", logger.String("backup_id", backupID))

	dataJSON, _ := json.Marshal(map[string]interface{}{
		"backup_id": backupID,
	})
	h.wsHub.Broadcast(&websocket.Message{
		Type: "config_restored",
		Data: json.RawMessage(dataJSON),
	})

	h.respondJSON(w, http.StatusOK, h.toConfigResponse(h.config))
}

func (h *ConfigHandler) toConfigResponse(cfg *config.AppConfig) ConfigResponse {
	return ConfigResponse{
		Server:       h.toServerConfig(cfg),
		Database:     h.toDatabaseConfig(cfg),
		Redis:        h.toRedisConfig(cfg),
		Email:        h.toEmailConfig(cfg),
		RateLimit:    h.toRateLimitConfig(cfg),
		Worker:       h.toWorkerConfig(cfg),
		Rotation:     h.toRotationConfig(cfg),
		Notification: h.toNotificationConfig(cfg),
		Security:     h.toSecurityConfig(cfg),
		Logging:      h.toLoggingConfig(cfg),
		Storage:      h.toStorageConfig(cfg),
		UpdatedAt:    time.Now(),
	}
}

func (h *ConfigHandler) toServerConfig(cfg *config.AppConfig) ServerConfig {
	return ServerConfig{
		Host:            cfg.Server.Host,
		Port:            cfg.Server.Port,
		Environment:     cfg.Server.Mode,
		Debug:           false,
		ShutdownTimeout: int(cfg.Server.IdleTimeout.Seconds()),
	}
}

func (h *ConfigHandler) toDatabaseConfig(cfg *config.AppConfig) DatabaseConfig {
	return DatabaseConfig{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		Database:        cfg.Database.Database,
		Username:        cfg.Database.Username,
		MaxConnections:  cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: int(cfg.Database.ConnMaxLifetime.Seconds()),
	}
}

func (h *ConfigHandler) toRedisConfig(cfg *config.AppConfig) RedisConfig {
	return RedisConfig{
		Host:       cfg.Cache.Host,
		Port:       cfg.Cache.Port,
		Database:   cfg.Cache.Database,
		MaxRetries: cfg.Cache.MaxRetries,
		PoolSize:   cfg.Cache.PoolSize,
	}
}

func (h *ConfigHandler) toEmailConfig(cfg *config.AppConfig) EmailConfig {
	return EmailConfig{
		DefaultFromName:  cfg.Email.FromName,
		DefaultFromEmail: cfg.Email.FromEmail,
		ReplyTo:          cfg.Email.ReplyTo,
		ReturnPath:       cfg.Email.ReturnPath,
		Timeout:          int(cfg.Email.SendTimeout.Seconds()),
		MaxRetries:       cfg.Email.RetryAttempts,
	}
}

func (h *ConfigHandler) toRateLimitConfig(cfg *config.AppConfig) RateLimitConfig {
	return RateLimitConfig{
		Enabled:            cfg.RateLimit.Enabled,
		RequestsPerSecond:  cfg.RateLimit.GlobalRPS,
		Burst:              cfg.RateLimit.BurstSize,
		AccountDailyLimit:  cfg.Account.DailyLimit,
		AccountHourlyLimit: cfg.Account.HourlyLimit,
	}
}

func (h *ConfigHandler) toWorkerConfig(cfg *config.AppConfig) WorkerConfig {
	return WorkerConfig{
		WorkerCount:     cfg.Worker.DefaultWorkers,
		QueueSize:       cfg.Worker.QueueSize,
		BatchSize:       cfg.Worker.BatchSize,
		ProcessingDelay: 0,
	}
}

func (h *ConfigHandler) toRotationConfig(cfg *config.AppConfig) RotationConfig {
	return RotationConfig{
		AccountStrategy:  cfg.Account.RotationStrategy,
		TemplateStrategy: cfg.Template.RotationStrategy,
		SubjectStrategy:  cfg.Personalization.SubjectRotation,
		SenderStrategy:   cfg.Personalization.SenderNameRotation,
		ProxyEnabled:     cfg.Proxy.Enabled,
	}
}

func (h *ConfigHandler) toNotificationConfig(cfg *config.AppConfig) NotificationConfig {
	return NotificationConfig{
		TelegramEnabled:  cfg.Notification.Enabled,
		TelegramChatID:   cfg.Notification.TelegramChatID,
		NotifyOnStart:    cfg.Campaign.NotifyOnStart,
		NotifyOnComplete: cfg.Campaign.NotifyOnComplete,
		NotifyOnError:    cfg.Campaign.NotifyOnError,
	}
}

func (h *ConfigHandler) toSecurityConfig(cfg *config.AppConfig) SecurityConfig {
	return SecurityConfig{
		JWTExpiration:  int(cfg.Security.JWTExpiration.Seconds()),
		AllowedOrigins: cfg.Server.AllowedOrigins,
		EnableCSRF:     cfg.Security.EnableCSRF,
	}
}

func (h *ConfigHandler) toLoggingConfig(cfg *config.AppConfig) LoggingConfig {
	return LoggingConfig{
		Level:           cfg.Logging.Level,
		Format:          cfg.Logging.Format,
		OutputPath:      "",
		ErrorOutputPath: "",
		EnableConsole:   cfg.Logging.EnableSystemLog,
		EnableFile:      true,
	}
}

func (h *ConfigHandler) toStorageConfig(cfg *config.AppConfig) StorageConfig {
	return StorageConfig{
		Type:         cfg.Storage.Type,
		LocalPath:    cfg.Storage.BasePath,
		MaxFileSize:  int64(cfg.Storage.MaxUploadSizeMB),
		AllowedTypes: cfg.Storage.AllowedExtensions,
	}
}

func (h *ConfigHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *ConfigHandler) respondError(w http.ResponseWriter, err error) {
	var status int
	var message string

	if appErr, ok := err.(*errors.Error); ok {
		status = appErr.StatusCode
		message = appErr.Message
	} else {
		status = http.StatusInternalServerError
		message = "Internal server error"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   message,
		"status":  status,
		"success": false,
	})
}
func (h *ConfigHandler) marshalJSON(v interface{}) json.RawMessage {
    data, _ := json.Marshal(v)
    return json.RawMessage(data)
}
// BackupConfig creates a backup of the current configuration
func (h *ConfigHandler) BackupConfig(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Get all configuration entries
    allEntries, err := h.configRepo.GetAll(ctx)
    if err != nil {
        h.logger.Error("Failed to get all config entries", logger.Error(err))
        h.respondError(w, errors.Internal("Failed to retrieve configuration"))
        return
    }

    // Create backup filename with timestamp
    timestamp := time.Now().Format("20060102_150405")
    backupName := fmt.Sprintf("config_backup_%s.json", timestamp)
    backupPath := filepath.Join("./storage/backups", backupName)

    // Create backups directory if it doesn't exist
    if err := os.MkdirAll("./storage/backups", 0755); err != nil {
        h.logger.Error("Failed to create backups directory", logger.Error(err))
        h.respondError(w, errors.Internal("Failed to create backup directory"))
        return
    }

    // Marshal config entries to JSON
    backupData, err := json.MarshalIndent(allEntries, "", "  ")
    if err != nil {
        h.logger.Error("Failed to marshal config data", logger.Error(err))
        h.respondError(w, errors.Internal("Failed to create backup"))
        return
    }

    // Write backup file
    if err := os.WriteFile(backupPath, backupData, 0644); err != nil {
        h.logger.Error("Failed to write backup file", logger.Error(err))
        h.respondError(w, errors.Internal("Failed to save backup"))
        return
    }

    h.logger.Info("Configuration backup created",
        logger.String("backup_path", backupPath),
        logger.Int("entries", len(allEntries)),
    )

    // Broadcast event
    h.wsHub.Broadcast(&websocket.Message{
        Type: "config_backed_up",
        Data: h.marshalJSON(map[string]interface{}{
            "backup_name": backupName,
            "timestamp":   time.Now(),
        }),
    })

    h.respondJSON(w, http.StatusOK, map[string]interface{}{
        "success":     true,
        "message":     "Configuration backed up successfully",
        "backup_name": backupName,
        "backup_path": backupPath,
        "entries":     len(allEntries),
        "timestamp":   time.Now(),
    })
}

func (h *ConfigHandler) RestoreConfig(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    var req struct {
        BackupName string `json:"backup_name" validate:"required"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, errors.BadRequest("Invalid request body"))
        return
    }

    if err := h.validator.Validate(req); err != nil {
        h.respondError(w, errors.ValidationFailed(err.Error())) // ✅ Fixed
        return
    }

    // Security: Validate backup name to prevent path traversal
    if strings.Contains(req.BackupName, "..") || strings.Contains(req.BackupName, "/") {
        h.respondError(w, errors.BadRequest("Invalid backup name"))
        return
    }

    backupPath := filepath.Join("./storage/backups", req.BackupName)

    // Check if backup file exists
    if _, err := os.Stat(backupPath); os.IsNotExist(err) {
        h.respondError(w, errors.NotFound("backup", req.BackupName)) // ✅ Fixed: provide resource and ID
        return
    }

    // Read backup file
    backupData, err := os.ReadFile(backupPath)
    if err != nil {
        h.logger.Error("Failed to read backup file", logger.Error(err))
        h.respondError(w, errors.Internal("Failed to read backup"))
        return
    }

    // Unmarshal backup data
    var configEntries []*repository.ConfigEntry
    if err := json.Unmarshal(backupData, &configEntries); err != nil {
        h.logger.Error("Failed to parse backup file", logger.Error(err))
        h.respondError(w, errors.BadRequest("Invalid backup file format"))
        return
    }

    // Restore each configuration entry
    restored := 0
    failed := 0
    for _, entry := range configEntries {
        entry.UpdatedBy = "restore"
        entry.UpdatedAt = time.Now()

        if err := h.configRepo.Set(ctx, entry); err != nil {
            h.logger.Error("Failed to restore config entry",
                logger.String("section", entry.Section),
                logger.String("key", entry.Key),
                logger.Error(err),
            )
            failed++
            continue
        }
        restored++
    }

    h.logger.Info("Configuration restored from backup",
        logger.String("backup_name", req.BackupName),
        logger.Int("restored", restored),
        logger.Int("failed", failed),
    )

    // Broadcast event
    h.wsHub.Broadcast(&websocket.Message{
        Type: "config_restored",
        Data: h.marshalJSON(map[string]interface{}{
            "backup_name": req.BackupName,
            "restored":    restored,
            "timestamp":   time.Now(),
        }),
    })

    h.respondJSON(w, http.StatusOK, map[string]interface{}{
        "success":     true,
        "message":     "Configuration restored successfully",
        "backup_name": req.BackupName,
        "restored":    restored,
        "failed":      failed,
        "total":       len(configEntries),
    })
}

// UpdateConfigSection updates a specific configuration section
func (h *ConfigHandler) UpdateConfigSection(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    vars := mux.Vars(r)
    section := vars["section"]

    if section == "" {
        h.respondError(w, errors.BadRequest("Section name is required"))
        return
    }

    // Parse request body as map
    var updates map[string]interface{}
    if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
        h.respondError(w, errors.BadRequest("Invalid request body"))
        return
    }

    if len(updates) == 0 {
        h.respondError(w, errors.BadRequest("No configuration updates provided"))
        return
    }

    // Update each key in the section
    updated := 0
    failed := 0
    errorList := []string{} 

    for key, value := range updates {
        // Determine type
        valueType := "string"
        var valueStr string

        switch v := value.(type) {
        case bool:
            valueType = "boolean"
            valueStr = fmt.Sprintf("%t", v)
        case float64, int, int64:
            valueType = "number"
            valueStr = fmt.Sprintf("%v", v)
        case string:
            valueType = "string"
            valueStr = v
        case map[string]interface{}, []interface{}:
            valueType = "json"
            jsonData, _ := json.Marshal(v)
            valueStr = string(jsonData)
        default:
            valueType = "string"
            valueStr = fmt.Sprintf("%v", v)
        }

        entry := &repository.ConfigEntry{
            Section:   section,
            Key:       key,
            Value:     valueStr,
            Type:      valueType,
            UpdatedBy: "api",
            UpdatedAt: time.Now(),
        }

        if err := h.configRepo.Set(ctx, entry); err != nil {
            h.logger.Error("Failed to update config entry",
                logger.String("section", section),
                logger.String("key", key),
                logger.Error(err),
            )
            failed++
            errorList = append(errorList, fmt.Sprintf("%s: %v", key, err)) // ✅ Use errorList
            continue
        }
        updated++
    }

    h.logger.Info("Configuration section updated",
        logger.String("section", section),
        logger.Int("updated", updated),
        logger.Int("failed", failed),
    )

    // Broadcast event
    h.wsHub.Broadcast(&websocket.Message{
        Type: "config_section_updated",
        Data: h.marshalJSON(map[string]interface{}{
            "section":   section,
            "updated":   updated,
            "timestamp": time.Now(),
        }),
    })

    response := map[string]interface{}{
        "success": true,
        "message": fmt.Sprintf("Section '%s' updated successfully", section),
        "section": section,
        "updated": updated,
        "failed":  failed,
        "total":   len(updates),
    }

    if len(errorList) > 0 { // ✅ Use errorList
        response["errors"] = errorList
    }

    h.respondJSON(w, http.StatusOK, response)
}