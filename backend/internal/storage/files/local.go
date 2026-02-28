package files

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type LocalStorage struct {
	config     *StorageConfig
	mu         sync.RWMutex
	cleanupMu  sync.Mutex
	stopChan   chan struct{}
	cleanTimer *time.Timer
}

func NewLocalStorage(config *StorageConfig) (*LocalStorage, error) {
	if config == nil {
		config = DefaultStorageConfig()
	}

	if err := validatePath(config.BasePath); err != nil {
		return nil, &FileError{Op: "init", Path: config.BasePath, Err: err}
	}

	if err := os.MkdirAll(config.BasePath, config.Permissions); err != nil {
		return nil, &FileError{Op: "mkdir", Path: config.BasePath, Err: err}
	}

	if config.TempDir != "" {
		if err := os.MkdirAll(config.TempDir, config.Permissions); err != nil {
			return nil, &FileError{Op: "mkdir", Path: config.TempDir, Err: err}
		}
	}

	ls := &LocalStorage{
		config:   config,
		stopChan: make(chan struct{}),
	}

	if config.AutoClean {
		ls.startCleanupLoop()
	}

	return ls, nil
}

func (ls *LocalStorage) Write(ctx context.Context, path string, data []byte) error {
	if err := ls.validatePath(path); err != nil {
		return &FileError{Op: "write", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, ls.config.Permissions); err != nil {
		return &FileError{Op: "mkdir", Path: dir, Err: err}
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return &FileError{Op: "write", Path: path, Err: err}
	}

	return nil
}

func (ls *LocalStorage) WriteReader(ctx context.Context, path string, reader io.Reader) error {
	if err := ls.validatePath(path); err != nil {
		return &FileError{Op: "write", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, ls.config.Permissions); err != nil {
		return &FileError{Op: "mkdir", Path: dir, Err: err}
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return &FileError{Op: "create", Path: path, Err: err}
	}
	defer file.Close()

	written, err := io.Copy(file, reader)
	if err != nil {
		os.Remove(fullPath)
		return &FileError{Op: "write", Path: path, Err: err}
	}

	if ls.config.MaxFileSize > 0 && written > ls.config.MaxFileSize {
		os.Remove(fullPath)
		return &FileError{Op: "write", Path: path, Err: fmt.Errorf("file size exceeds limit")}
	}

	return nil
}

func (ls *LocalStorage) Read(ctx context.Context, path string) ([]byte, error) {
	if err := ls.validatePath(path); err != nil {
		return nil, &FileError{Op: "read", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, &FileError{Op: "read", Path: path, Err: err}
	}

	return data, nil
}

func (ls *LocalStorage) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	if err := ls.validatePath(path); err != nil {
		return nil, &FileError{Op: "read", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, &FileError{Op: "open", Path: path, Err: err}
	}

	return file, nil
}

func (ls *LocalStorage) Delete(ctx context.Context, path string) error {
	if err := ls.validatePath(path); err != nil {
		return &FileError{Op: "delete", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return &FileError{Op: "delete", Path: path, Err: err}
	}

	return nil
}

func (ls *LocalStorage) Exists(ctx context.Context, path string) (bool, error) {
	if err := ls.validatePath(path); err != nil {
		return false, &FileError{Op: "exists", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, &FileError{Op: "stat", Path: path, Err: err}
}

func (ls *LocalStorage) Size(ctx context.Context, path string) (int64, error) {
	if err := ls.validatePath(path); err != nil {
		return 0, &FileError{Op: "size", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	info, err := os.Stat(fullPath)
	if err != nil {
		return 0, &FileError{Op: "stat", Path: path, Err: err}
	}

	return info.Size(), nil
}

func (ls *LocalStorage) Copy(ctx context.Context, src, dst string) error {
	if err := ls.validatePath(src); err != nil {
		return &FileError{Op: "copy", Path: src, Err: err}
	}
	if err := ls.validatePath(dst); err != nil {
		return &FileError{Op: "copy", Path: dst, Err: err}
	}

	srcPath := ls.getFullPath(src)
	dstPath := ls.getFullPath(dst)

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return &FileError{Op: "open", Path: src, Err: err}
	}
	defer srcFile.Close()

	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, ls.config.Permissions); err != nil {
		return &FileError{Op: "mkdir", Path: dstDir, Err: err}
	}

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return &FileError{Op: "create", Path: dst, Err: err}
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dstPath)
		return &FileError{Op: "copy", Path: dst, Err: err}
	}

	return nil
}

func (ls *LocalStorage) Move(ctx context.Context, src, dst string) error {
	if err := ls.validatePath(src); err != nil {
		return &FileError{Op: "move", Path: src, Err: err}
	}
	if err := ls.validatePath(dst); err != nil {
		return &FileError{Op: "move", Path: dst, Err: err}
	}

	srcPath := ls.getFullPath(src)
	dstPath := ls.getFullPath(dst)

	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, ls.config.Permissions); err != nil {
		return &FileError{Op: "mkdir", Path: dstDir, Err: err}
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		return &FileError{Op: "move", Path: src, Err: err}
	}

	return nil
}

func (ls *LocalStorage) List(ctx context.Context, dirPath string) ([]*FileInfo, error) {
	if err := ls.validatePath(dirPath); err != nil {
		return nil, &FileError{Op: "list", Path: dirPath, Err: err}
	}

	fullPath := ls.getFullPath(dirPath)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, &FileError{Op: "readdir", Path: dirPath, Err: err}
	}

	var files []*FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		entryPath := filepath.Join(dirPath, entry.Name())
		fileInfo := ls.buildFileInfo(entryPath, info)
		files = append(files, fileInfo)
	}

	return files, nil
}

func (ls *LocalStorage) CreateDir(ctx context.Context, dirPath string) error {
	if err := ls.validatePath(dirPath); err != nil {
		return &FileError{Op: "mkdir", Path: dirPath, Err: err}
	}

	fullPath := ls.getFullPath(dirPath)
	if err := os.MkdirAll(fullPath, ls.config.Permissions); err != nil {
		return &FileError{Op: "mkdir", Path: dirPath, Err: err}
	}

	return nil
}

func (ls *LocalStorage) DeleteDir(ctx context.Context, dirPath string) error {
	if err := ls.validatePath(dirPath); err != nil {
		return &FileError{Op: "rmdir", Path: dirPath, Err: err}
	}

	fullPath := ls.getFullPath(dirPath)
	if err := os.RemoveAll(fullPath); err != nil {
		return &FileError{Op: "rmdir", Path: dirPath, Err: err}
	}

	return nil
}

func (ls *LocalStorage) GetInfo(ctx context.Context, path string) (*FileInfo, error) {
	if err := ls.validatePath(path); err != nil {
		return nil, &FileError{Op: "stat", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, &FileError{Op: "stat", Path: path, Err: err}
	}

	return ls.buildFileInfo(path, info), nil
}

func (ls *LocalStorage) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	if err := ls.validatePath(path); err != nil {
		return &FileError{Op: "chmod", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	if err := os.Chmod(fullPath, mode); err != nil {
		return &FileError{Op: "chmod", Path: path, Err: err}
	}

	return nil
}

func (ls *LocalStorage) GetTempDir(ctx context.Context) (string, error) {
	if ls.config.TempDir == "" {
		return "", fmt.Errorf("temp directory not configured")
	}

	tempDir := filepath.Join(ls.config.TempDir, fmt.Sprintf("tmp_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, ls.config.Permissions); err != nil {
		return "", &FileError{Op: "mkdir", Path: tempDir, Err: err}
	}

	return tempDir, nil
}

func (ls *LocalStorage) Clean(ctx context.Context, path string, olderThan time.Duration) error {
	if err := ls.validatePath(path); err != nil {
		return &FileError{Op: "clean", Path: path, Err: err}
	}

	fullPath := ls.getFullPath(path)
	cutoff := time.Now().Add(-olderThan)

	return filepath.Walk(fullPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() && info.ModTime().Before(cutoff) {
			os.Remove(p)
		}

		return nil
	})
}

func (ls *LocalStorage) TotalSize(ctx context.Context, dirPath string) (int64, error) {
	if err := ls.validatePath(dirPath); err != nil {
		return 0, &FileError{Op: "size", Path: dirPath, Err: err}
	}

	fullPath := ls.getFullPath(dirPath)
	var totalSize int64

	err := filepath.Walk(fullPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	if err != nil {
		return 0, &FileError{Op: "walk", Path: dirPath, Err: err}
	}

	return totalSize, nil
}

func (ls *LocalStorage) Close() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.cleanTimer != nil {
		ls.cleanTimer.Stop()
	}

	close(ls.stopChan)
	return nil
}

func (ls *LocalStorage) getFullPath(path string) string {
	return filepath.Join(ls.config.BasePath, filepath.Clean(path))
}

func (ls *LocalStorage) validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") {
		return fmt.Errorf("path traversal detected")
	}

	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths not allowed")
	}

	return nil
}

func (ls *LocalStorage) buildFileInfo(path string, info os.FileInfo) *FileInfo {
	ext := filepath.Ext(path)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return &FileInfo{
		Name:        info.Name(),
		Path:        path,
		Size:        info.Size(),
		Mode:        info.Mode(),
		ModTime:     info.ModTime(),
		IsDir:       info.IsDir(),
		MimeType:    mimeType,
		Extension:   ext,
		Permissions: info.Mode().String(),
	}
}

func (ls *LocalStorage) startCleanupLoop() {
	go func() {
		ticker := time.NewTicker(ls.config.CleanInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ls.performCleanup()
			case <-ls.stopChan:
				return
			}
		}
	}()
}

func (ls *LocalStorage) performCleanup() {
	ls.cleanupMu.Lock()
	defer ls.cleanupMu.Unlock()

	if ls.config.TempDir == "" {
		return
	}

	ctx := context.Background()
	ls.Clean(ctx, ls.config.TempDir, ls.config.CleanAge)
}

func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path contains invalid elements")
	}

	return nil
}

func ListWithOptions(ls *LocalStorage, ctx context.Context, dirPath string, opts *ListOptions) ([]*FileInfo, error) {
	if opts == nil {
		opts = DefaultListOptions()
	}

	files, err := ls.List(ctx, dirPath)
	if err != nil {
		return nil, err
	}

	filtered := make([]*FileInfo, 0, len(files))
	for _, file := range files {
		if !opts.IncludeDirs && file.IsDir {
			continue
		}

		if opts.Pattern != "*" {
			matched, _ := filepath.Match(opts.Pattern, file.Name)
			if !matched {
				continue
			}
		}

		filtered = append(filtered, file)
	}

	sortFiles(filtered, opts.SortBy, opts.SortOrder)

	start := opts.Offset
	if start > len(filtered) {
		start = len(filtered)
	}

	end := start + opts.Limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[start:end], nil
}

func sortFiles(files []*FileInfo, sortBy, sortOrder string) {
	sort.Slice(files, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "size":
			less = files[i].Size < files[j].Size
		case "modified":
			less = files[i].ModTime.Before(files[j].ModTime)
		default:
			less = files[i].Name < files[j].Name
		}

		if sortOrder == "desc" {
			return !less
		}
		return less
	})
}

func ComputeHash(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

func ComputeFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
