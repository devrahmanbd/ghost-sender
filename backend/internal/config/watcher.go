package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type ChangeEvent struct {
	Path      string
	Operation string
	Timestamp time.Time
	OldConfig *AppConfig
	NewConfig *AppConfig
}

type ChangeHandler func(*ChangeEvent) error

type Watcher struct {
	mu              sync.RWMutex
	config          *AppConfig
	configPath      string
	watcher         *fsnotify.Watcher
	handlers        []ChangeHandler
	enabled         bool
	running         bool
	debounceTime    time.Duration
	lastReload      time.Time
	reloadChan      chan string
	stopChan        chan struct{}
	errorChan       chan error
	ctx             context.Context
	cancel          context.CancelFunc
	watchedFiles    map[string]bool
	reloadInProgress bool
}

func NewWatcher(cfg *AppConfig, configPath string) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := &Watcher{
		config:       cfg,
		configPath:   configPath,
		watcher:      fsWatcher,
		handlers:     []ChangeHandler{},
		enabled:      true,
		debounceTime: 1 * time.Second,
		reloadChan:   make(chan string, 10),
		stopChan:     make(chan struct{}),
		errorChan:    make(chan error, 10),
		ctx:          ctx,
		cancel:       cancel,
		watchedFiles: make(map[string]bool),
	}

	return w, nil
}

func (w *Watcher) Start() error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("watcher already running")
	}
	w.running = true
	w.mu.Unlock()

	if err := w.addWatchPath(w.configPath); err != nil {
		return fmt.Errorf("failed to watch config file: %w", err)
	}

	configDir := filepath.Dir(w.configPath)
	providersPath := filepath.Join(configDir, "providers.yaml")
	if _, err := os.Stat(providersPath); err == nil {
		w.addWatchPath(providersPath)
	}

	rotationPath := filepath.Join(configDir, "rotation.yaml")
	if _, err := os.Stat(rotationPath); err == nil {
		w.addWatchPath(rotationPath)
	}

	go w.watchLoop()
	go w.reloadLoop()

	return nil
}

func (w *Watcher) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false
	w.mu.Unlock()

	w.cancel()
	close(w.stopChan)

	if w.watcher != nil {
		return w.watcher.Close()
	}

	return nil
}

func (w *Watcher) OnChange(handler ChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, handler)
}

func (w *Watcher) RemoveHandler(handler ChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, h := range w.handlers {
		if &h == &handler {
			w.handlers = append(w.handlers[:i], w.handlers[i+1:]...)
			break
		}
	}
}

func (w *Watcher) GetConfig() *AppConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.config.Clone()
}

func (w *Watcher) SetDebounceTime(duration time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.debounceTime = duration
}

func (w *Watcher) Enable() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.enabled = true
}

func (w *Watcher) Disable() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.enabled = false
}

func (w *Watcher) IsEnabled() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.enabled
}

func (w *Watcher) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

func (w *Watcher) AddWatchPath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.addWatchPath(path)
}

func (w *Watcher) addWatchPath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	if w.watchedFiles[absPath] {
		return nil
	}

	if err := w.watcher.Add(absPath); err != nil {
		return fmt.Errorf("failed to add watch path: %w", err)
	}

	w.watchedFiles[absPath] = true
	return nil
}

func (w *Watcher) RemoveWatchPath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	if !w.watchedFiles[absPath] {
		return nil
	}

	if err := w.watcher.Remove(absPath); err != nil {
		return fmt.Errorf("failed to remove watch path: %w", err)
	}

	delete(w.watchedFiles, absPath)
	return nil
}

func (w *Watcher) watchLoop() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.stopChan:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleFileEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.handleError(err)
		}
	}
}

func (w *Watcher) reloadLoop() {
	var timer *time.Timer
	var pendingPath string

	for {
		select {
		case <-w.ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-w.stopChan:
			if timer != nil {
				timer.Stop()
			}
			return
		case path := <-w.reloadChan:
			pendingPath = path
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(w.debounceTime, func() {
				w.performReload(pendingPath)
			})
		}
	}
}

func (w *Watcher) handleFileEvent(event fsnotify.Event) {
	if !w.IsEnabled() {
		return
	}

	if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
		select {
		case w.reloadChan <- event.Name:
		default:
		}
	}

	if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		w.mu.Lock()
		delete(w.watchedFiles, event.Name)
		w.mu.Unlock()

		time.AfterFunc(100*time.Millisecond, func() {
			if _, err := os.Stat(event.Name); err == nil {
				w.AddWatchPath(event.Name)
			}
		})
	}
}

func (w *Watcher) handleError(err error) {
	select {
	case w.errorChan <- err:
	default:
	}
}

func (w *Watcher) performReload(path string) {
	w.mu.Lock()
	if w.reloadInProgress {
		w.mu.Unlock()
		return
	}
	w.reloadInProgress = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.reloadInProgress = false
		w.lastReload = time.Now()
		w.mu.Unlock()
	}()

	if time.Since(w.lastReload) < w.debounceTime {
		return
	}

	oldConfig := w.GetConfig()

	newConfig, err := w.loadConfig(path)
	if err != nil {
		w.handleError(fmt.Errorf("failed to load config: %w", err))
		return
	}

	if err := ValidateConfig(newConfig); err != nil {
		w.handleError(fmt.Errorf("config validation failed: %w", err))
		return
	}

	event := &ChangeEvent{
		Path:      path,
		Operation: "reload",
		Timestamp: time.Now(),
		OldConfig: oldConfig,
		NewConfig: newConfig,
	}

	if err := w.notifyHandlers(event); err != nil {
		w.handleError(fmt.Errorf("handler notification failed: %w", err))
		return
	}

	w.mu.Lock()
	w.config = newConfig
	w.mu.Unlock()
}

func (w *Watcher) loadConfig(path string) (*AppConfig, error) {
	loader := NewLoader()
	return loader.Load(path)
}

func (w *Watcher) notifyHandlers(event *ChangeEvent) error {
	w.mu.RLock()
	handlers := make([]ChangeHandler, len(w.handlers))
	copy(handlers, w.handlers)
	w.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(event); err != nil {
			return err
		}
	}

	return nil
}

func (w *Watcher) ForceReload() error {
	w.mu.Lock()
	if w.reloadInProgress {
		w.mu.Unlock()
		return fmt.Errorf("reload already in progress")
	}
	w.mu.Unlock()

	select {
	case w.reloadChan <- w.configPath:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("reload timeout")
	}
}

func (w *Watcher) GetLastReloadTime() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastReload
}

func (w *Watcher) GetWatchedFiles() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	files := make([]string, 0, len(w.watchedFiles))
	for path := range w.watchedFiles {
		files = append(files, path)
	}
	return files
}

func (w *Watcher) GetErrors() <-chan error {
	return w.errorChan
}

func WatchConfig(cfg *AppConfig, configPath string, handlers ...ChangeHandler) (*Watcher, error) {
	watcher, err := NewWatcher(cfg, configPath)
	if err != nil {
		return nil, err
	}

	for _, handler := range handlers {
		watcher.OnChange(handler)
	}

	if err := watcher.Start(); err != nil {
		return nil, err
	}

	return watcher, nil
}

func CreateDefaultHandler() ChangeHandler {
	return func(event *ChangeEvent) error {
		if event.NewConfig == nil {
			return fmt.Errorf("new config is nil")
		}
		return nil
	}
}

func CreateLoggingHandler(logger interface{}) ChangeHandler {
	return func(event *ChangeEvent) error {
		return nil
	}
}

func CreateValidationHandler() ChangeHandler {
	return func(event *ChangeEvent) error {
		if event.NewConfig == nil {
			return fmt.Errorf("config is nil")
		}
		return ValidateConfig(event.NewConfig)
	}
}

func CreateNotificationHandler(notifyFunc func(string) error) ChangeHandler {
	return func(event *ChangeEvent) error {
		message := fmt.Sprintf("Configuration reloaded from %s at %s",
			event.Path, event.Timestamp.Format(time.RFC3339))
		return notifyFunc(message)
	}
}

func CreateBackupHandler(backupPath string) ChangeHandler {
	return func(event *ChangeEvent) error {
		if event.OldConfig == nil {
			return nil
		}

		timestamp := event.Timestamp.Format("20060102-150405")
		filename := filepath.Join(backupPath, fmt.Sprintf("config-backup-%s.yaml", timestamp))

		if err := os.MkdirAll(backupPath, 0755); err != nil {
			return fmt.Errorf("failed to create backup directory: %w", err)
		}

		return event.OldConfig.SaveToFile(filename)
	}
}
func CreateRollbackHandler() ChangeHandler {
    var previousConfig *AppConfig

    return func(event *ChangeEvent) error {
        if event.OldConfig != nil {
            previousConfig = event.OldConfig.Clone()
            _ = previousConfig 
        }
        return nil
    }
}


func (w *Watcher) Rollback() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.config == nil {
		return fmt.Errorf("no config to rollback from")
	}

	oldConfig, err := w.loadConfig(w.configPath)
	if err != nil {
		return fmt.Errorf("failed to load previous config: %w", err)
	}

	if err := ValidateConfig(oldConfig); err != nil {
		return fmt.Errorf("previous config validation failed: %w", err)
	}

	w.config = oldConfig
	return nil
}

type WatcherStats struct {
	Enabled        bool
	Running        bool
	LastReload     time.Time
	WatchedFiles   []string
	HandlersCount  int
	DebounceTime   time.Duration
	ReloadInProgress bool
}

func (w *Watcher) GetStats() WatcherStats {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return WatcherStats{
		Enabled:          w.enabled,
		Running:          w.running,
		LastReload:       w.lastReload,
		WatchedFiles:     w.GetWatchedFiles(),
		HandlersCount:    len(w.handlers),
		DebounceTime:     w.debounceTime,
		ReloadInProgress: w.reloadInProgress,
	}
}

func (w *Watcher) ClearErrors() {
	for {
		select {
		case <-w.errorChan:
		default:
			return
		}
	}
}

func (w *Watcher) WaitForReload(timeout time.Duration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.Now().Add(timeout)

	for {
		select {
		case <-ticker.C:
			w.mu.RLock()
			inProgress := w.reloadInProgress
			w.mu.RUnlock()

			if !inProgress {
				return nil
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("reload timeout")
			}
		case <-w.ctx.Done():
			return fmt.Errorf("watcher stopped")
		}
	}
}

func (w *Watcher) SetConfig(cfg *AppConfig) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := ValidateConfig(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	w.config = cfg
	return nil
}

func (w *Watcher) CompareConfigs() (map[string]interface{}, error) {
	w.mu.RLock()
	oldConfig := w.config
	w.mu.RUnlock()

	newConfig, err := w.loadConfig(w.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load new config: %w", err)
	}

	differences := make(map[string]interface{})

	if oldConfig.App.Debug != newConfig.App.Debug {
		differences["app.debug"] = map[string]bool{
			"old": oldConfig.App.Debug,
			"new": newConfig.App.Debug,
		}
	}

	if oldConfig.Server.Port != newConfig.Server.Port {
		differences["server.port"] = map[string]int{
			"old": oldConfig.Server.Port,
			"new": newConfig.Server.Port,
		}
	}

	if oldConfig.Database.MaxOpenConns != newConfig.Database.MaxOpenConns {
		differences["database.max_open_conns"] = map[string]int{
			"old": oldConfig.Database.MaxOpenConns,
			"new": newConfig.Database.MaxOpenConns,
		}
	}

	if oldConfig.Worker.MaxWorkers != newConfig.Worker.MaxWorkers {
		differences["worker.max_workers"] = map[string]int{
			"old": oldConfig.Worker.MaxWorkers,
			"new": newConfig.Worker.MaxWorkers,
		}
	}

	return differences, nil
}

func (w *Watcher) ReloadSection(section string) error {
	newConfig, err := w.loadConfig(w.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := ValidateSection(newConfig, section); err != nil {
		return fmt.Errorf("section validation failed: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	switch section {
	case "worker":
		w.config.Worker = newConfig.Worker
	case "ratelimit":
		w.config.RateLimit = newConfig.RateLimit
	case "logging":
		w.config.Logging = newConfig.Logging
	default:
		return fmt.Errorf("section %s cannot be hot-reloaded", section)
	}

	return nil
}
