package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Loader struct {
	searchPaths []string
	envPrefix   string
}

func NewLoader() *Loader {
	return &Loader{
		searchPaths: []string{
			".",
			"./configs",
			"./config",
			"/etc/email-campaign",
			"$HOME/.email-campaign",
		},
		envPrefix: "",
	}
}

func (l *Loader) WithSearchPaths(paths ...string) *Loader {
	l.searchPaths = paths
	return l
}

func (l *Loader) WithEnvPrefix(prefix string) *Loader {
	l.envPrefix = prefix
	return l
}

func (l *Loader) Load(configPath string) (*AppConfig, error) {
	cfg := New()

	if err := cfg.LoadDefaults(); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	if configPath != "" {
		if err := l.loadYAMLFile(configPath, cfg); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	if err := l.loadProvidersConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to load providers config: %w", err)
	}

	if err := l.loadRotationConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to load rotation config: %w", err)
	}

	if err := l.loadEnvironmentVariables(cfg); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	cfg.loadedAt = time.Now()
	return cfg, nil
}

func (l *Loader) loadYAMLFile(path string, cfg *AppConfig) error {
	resolvedPath, err := l.resolvePath(path)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", resolvedPath, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	cfg.configPath = resolvedPath
	return nil
}

func (l *Loader) loadProvidersConfig(cfg *AppConfig) error {
	providerPaths := []string{
		"providers.yaml",
		"providers.yml",
		"configs/providers.yaml",
		"config/providers.yaml",
	}

	var providerPath string
	var err error

	for _, path := range providerPaths {
		providerPath, err = l.resolvePath(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil
	}

	data, err := os.ReadFile(providerPath)
	if err != nil {
		return nil
	}

	var providers map[string]ProviderConfig
	if err := yaml.Unmarshal(data, &providers); err != nil {
		return fmt.Errorf("failed to parse providers YAML: %w", err)
	}

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}

	for name, provider := range providers {
		cfg.Providers[name] = provider
	}

	return nil
}

func (l *Loader) loadRotationConfig(cfg *AppConfig) error {
	rotationPaths := []string{
		"rotation.yaml",
		"rotation.yml",
		"configs/rotation.yaml",
		"config/rotation.yaml",
	}

	var rotationPath string
	var err error

	for _, path := range rotationPaths {
		rotationPath, err = l.resolvePath(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil
	}

	data, err := os.ReadFile(rotationPath)
	if err != nil {
		return nil
	}

	var rotationStrategies RotationStrategyConfig
	if err := yaml.Unmarshal(data, &rotationStrategies); err != nil {
		return fmt.Errorf("failed to parse rotation YAML: %w", err)
	}

	cfg.RotationStrategies = rotationStrategies
	return nil
}

func (l *Loader) resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("file not found: %s", path)
	}

	for _, searchPath := range l.searchPaths {
		expandedPath := os.ExpandEnv(searchPath)
		fullPath := filepath.Join(expandedPath, path)

		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("file not found in search paths: %s", path)
}

func (l *Loader) loadEnvironmentVariables(cfg *AppConfig) error {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	l.loadEnvForStruct(reflect.ValueOf(&cfg.App).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Server).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Database).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Cache).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Storage).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Email).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Account).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Campaign).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Template).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Personalization).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Attachment).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Proxy).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.RateLimit).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Worker).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Notification).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Logging).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Security).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Monitoring).Elem(), "")
	l.loadEnvForStruct(reflect.ValueOf(&cfg.Cleanup).Elem(), "")

	return nil
}

func (l *Loader) loadEnvForStruct(v reflect.Value, prefix string) {
	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		if !field.CanSet() {
			continue
		}

		envTag := fieldType.Tag.Get("env")
		if envTag == "" {
			continue
		}

		fullEnvKey := envTag
		if l.envPrefix != "" {
			fullEnvKey = l.envPrefix + "_" + envTag
		}

		envValue := os.Getenv(fullEnvKey)
		if envValue == "" {
			continue
		}

		l.setFieldValue(field, envValue)
	}
}

func (l *Loader) setFieldValue(field reflect.Value, value string) {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			if duration, err := time.ParseDuration(value); err == nil {
				field.SetInt(int64(duration))
			}
		} else {
			if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
				field.SetInt(intVal)
			}
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if uintVal, err := strconv.ParseUint(value, 10, 64); err == nil {
			field.SetUint(uintVal)
		}

	case reflect.Float32, reflect.Float64:
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			field.SetFloat(floatVal)
		}

	case reflect.Bool:
		boolVal := strings.ToLower(value) == "true" || value == "1"
		field.SetBool(boolVal)

	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			values := strings.Split(value, ",")
			for i := range values {
				values[i] = strings.TrimSpace(values[i])
			}
			field.Set(reflect.ValueOf(values))
		}

	case reflect.Map:
		if field.Type().Key().Kind() == reflect.String && field.Type().Elem().Kind() == reflect.String {
			m := make(map[string]string)
			pairs := strings.Split(value, ",")
			for _, pair := range pairs {
				kv := strings.SplitN(pair, "=", 2)
				if len(kv) == 2 {
					m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				}
			}
			field.Set(reflect.ValueOf(m))
		}
	}
}

func LoadFromFile(path string) (*AppConfig, error) {
	loader := NewLoader()
	return loader.Load(path)
}

func LoadFromFileWithPaths(path string, searchPaths ...string) (*AppConfig, error) {
	loader := NewLoader().WithSearchPaths(searchPaths...)
	return loader.Load(path)
}

func LoadWithEnvPrefix(path, prefix string) (*AppConfig, error) {
	loader := NewLoader().WithEnvPrefix(prefix)
	return loader.Load(path)
}

func (c *AppConfig) SaveToFile(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := c.backupConfig(path); err != nil {
		return fmt.Errorf("failed to backup config: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (c *AppConfig) backupConfig(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	backupPath := path + ".backup." + time.Now().Format("20060102-150405")

	source, err := os.Open(path)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func (c *AppConfig) ExportToYAML() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return yaml.Marshal(c)
}

func (c *AppConfig) ImportFromYAML(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return yaml.Unmarshal(data, c)
}

func MergeConfigs(base, override *AppConfig) *AppConfig {
	merged := base.Clone()

	if override.App.Name != "" {
		merged.App.Name = override.App.Name
	}
	if override.App.Version != "" {
		merged.App.Version = override.App.Version
	}
	if override.App.Environment != "" {
		merged.App.Environment = override.App.Environment
	}
	if override.App.TenantID != "" {
		merged.App.TenantID = override.App.TenantID
	}

	if override.Server.Host != "" {
		merged.Server.Host = override.Server.Host
	}
	if override.Server.Port != 0 {
		merged.Server.Port = override.Server.Port
	}

	if override.Database.Host != "" {
		merged.Database.Host = override.Database.Host
	}
	if override.Database.Port != 0 {
		merged.Database.Port = override.Database.Port
	}
	if override.Database.Database != "" {
		merged.Database.Database = override.Database.Database
	}
	if override.Database.Username != "" {
		merged.Database.Username = override.Database.Username
	}
	if override.Database.Password != "" {
		merged.Database.Password = override.Database.Password
	}

	if override.Cache.Host != "" {
		merged.Cache.Host = override.Cache.Host
	}
	if override.Cache.Port != 0 {
		merged.Cache.Port = override.Cache.Port
	}

	if override.Security.JWTSecret != "" {
		merged.Security.JWTSecret = override.Security.JWTSecret
	}
	if override.Security.APIKey != "" {
		merged.Security.APIKey = override.Security.APIKey
	}
	if override.Security.EncryptionKey != "" {
		merged.Security.EncryptionKey = override.Security.EncryptionKey
	}

	if override.Notification.TelegramBotToken != "" {
		merged.Notification.TelegramBotToken = override.Notification.TelegramBotToken
	}
	if override.Notification.TelegramChatID != "" {
		merged.Notification.TelegramChatID = override.Notification.TelegramChatID
	}

	for name, provider := range override.Providers {
		merged.Providers[name] = provider
	}

	merged.UpdatedAt = time.Now()
	return merged
}

func ParseConfigFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return result, nil
}

func ValidateConfigFile(path string) error {
	cfg := New()
	if err := cfg.LoadDefaults(); err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	return cfg.Validate()
}

func GetConfigTemplate() string {
	return `app:
  name: "Email Campaign System"
  version: "1.0.0"
  environment: "production"
  debug: false
  tenant_id: ""
  timezone: "UTC"
  locale: "en"

server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"
  tls_enabled: false
  read_timeout: 10s
  write_timeout: 10s
  idle_timeout: 120s
  enable_cors: true
  enable_websocket: true

database:
  driver: "postgres"
  host: "localhost"
  port: 5432
  database: "email_campaign"
  username: "postgres"
  password: ""
  ssl_mode: "disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m

cache:
  type: "redis"
  host: "localhost"
  port: 6379
  password: ""
  database: 0
  pool_size: 10
  default_expiration: 1h

email:
  default_charset: "UTF-8"
  send_timeout: 30s
  retry_attempts: 3
  enable_fbl: true
  enable_unsubscribe: true

account:
  rotation_strategy: "round-robin"
  rotation_limit: 100
  daily_limit: 500
  auto_suspend: true
  consecutive_failures: 5

campaign:
  max_concurrent: 10
  enable_checkpointing: true
  checkpoint_interval: 1m
  enable_notifications: true

worker:
  min_workers: 1
  max_workers: 10
  default_workers: 4
  queue_size: 1000
  batch_size: 100

logging:
  level: "info"
  format: "json"
  max_size_mb: 100
  max_backups: 10
  compress: true

security:
  enable_auth: true
  enable_encryption: true
  enable_csrf: true
  jwt_expiration: 24h
`
}

func GenerateExampleConfig(path string) error {
	template := GetConfigTemplate()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ExpandEnvInConfig(cfg *AppConfig) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	cfg.Database.Host = os.ExpandEnv(cfg.Database.Host)
	cfg.Database.Username = os.ExpandEnv(cfg.Database.Username)
	cfg.Database.Password = os.ExpandEnv(cfg.Database.Password)

	cfg.Cache.Host = os.ExpandEnv(cfg.Cache.Host)
	cfg.Cache.Password = os.ExpandEnv(cfg.Cache.Password)

	cfg.Storage.BasePath = os.ExpandEnv(cfg.Storage.BasePath)
	cfg.Storage.TempPath = os.ExpandEnv(cfg.Storage.TempPath)
	cfg.Storage.UploadPath = os.ExpandEnv(cfg.Storage.UploadPath)
	cfg.Storage.TemplatePath = os.ExpandEnv(cfg.Storage.TemplatePath)
	cfg.Storage.AttachmentPath = os.ExpandEnv(cfg.Storage.AttachmentPath)
	cfg.Storage.LogPath = os.ExpandEnv(cfg.Storage.LogPath)
	cfg.Storage.BackupPath = os.ExpandEnv(cfg.Storage.BackupPath)

	cfg.Security.JWTSecret = os.ExpandEnv(cfg.Security.JWTSecret)
	cfg.Security.APIKey = os.ExpandEnv(cfg.Security.APIKey)
	cfg.Security.EncryptionKey = os.ExpandEnv(cfg.Security.EncryptionKey)

	cfg.Notification.TelegramBotToken = os.ExpandEnv(cfg.Notification.TelegramBotToken)
	cfg.Notification.TelegramChatID = os.ExpandEnv(cfg.Notification.TelegramChatID)
}

func LoadMultipleConfigs(paths ...string) (*AppConfig, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no config paths provided")
	}

	loader := NewLoader()
	base, err := loader.Load(paths[0])
	if err != nil {
		return nil, err
	}

	for i := 1; i < len(paths); i++ {
		override, err := loader.Load(paths[i])
		if err != nil {
			continue
		}
		base = MergeConfigs(base, override)
	}

	return base, nil
}

func (c *AppConfig) GetSection(section string) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	switch strings.ToLower(section) {
	case "app", "application":
		return c.App, nil
	case "server":
		return c.Server, nil
	case "database", "db":
		return c.Database, nil
	case "cache":
		return c.Cache, nil
	case "storage":
		return c.Storage, nil
	case "email":
		return c.Email, nil
	case "account":
		return c.Account, nil
	case "campaign":
		return c.Campaign, nil
	case "template":
		return c.Template, nil
	case "personalization":
		return c.Personalization, nil
	case "attachment":
		return c.Attachment, nil
	case "proxy":
		return c.Proxy, nil
	case "ratelimit", "rate_limit":
		return c.RateLimit, nil
	case "worker":
		return c.Worker, nil
	case "notification":
		return c.Notification, nil
	case "logging", "log":
		return c.Logging, nil
	case "security":
		return c.Security, nil
	case "monitoring":
		return c.Monitoring, nil
	case "cleanup":
		return c.Cleanup, nil
	default:
		return nil, fmt.Errorf("unknown section: %s", section)
	}
}

func (c *AppConfig) UpdateSection(section string, data interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch strings.ToLower(section) {
	case "app", "application":
		if v, ok := data.(ApplicationConfig); ok {
			c.App = v
		}
	case "server":
		if v, ok := data.(ServerConfig); ok {
			c.Server = v
		}
	case "database", "db":
		if v, ok := data.(DatabaseConfig); ok {
			c.Database = v
		}
	case "cache":
		if v, ok := data.(CacheConfig); ok {
			c.Cache = v
		}
	case "email":
		if v, ok := data.(EmailConfig); ok {
			c.Email = v
		}
	case "security":
		if v, ok := data.(SecurityConfig); ok {
			c.Security = v
		}
	default:
		return fmt.Errorf("unknown or unsupported section: %s", section)
	}

	c.UpdatedAt = time.Now()
	return nil
}
