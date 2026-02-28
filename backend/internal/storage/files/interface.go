package files

import (
	"context"
	"io"
	"os"
	"time"
)

type Storage interface {
	Write(ctx context.Context, path string, data []byte) error
	WriteReader(ctx context.Context, path string, reader io.Reader) error
	Read(ctx context.Context, path string) ([]byte, error)
	ReadStream(ctx context.Context, path string) (io.ReadCloser, error)
	Delete(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) (bool, error)
	Size(ctx context.Context, path string) (int64, error)
	Copy(ctx context.Context, src, dst string) error
	Move(ctx context.Context, src, dst string) error
	List(ctx context.Context, dirPath string) ([]*FileInfo, error)
	CreateDir(ctx context.Context, dirPath string) error
	DeleteDir(ctx context.Context, dirPath string) error
	GetInfo(ctx context.Context, path string) (*FileInfo, error)
	Chmod(ctx context.Context, path string, mode os.FileMode) error
	GetTempDir(ctx context.Context) (string, error)
	Clean(ctx context.Context, path string, olderThan time.Duration) error
	TotalSize(ctx context.Context, dirPath string) (int64, error)
	Close() error
}

type FileInfo struct {
	Name         string
	Path         string
	Size         int64
	Mode         os.FileMode
	ModTime      time.Time
	IsDir        bool
	MimeType     string
	Extension    string
	Hash         string
	Permissions  string
}

type StorageConfig struct {
	BasePath      string
	MaxFileSize   int64
	AllowedTypes  []string
	TempDir       string
	AutoClean     bool
	CleanInterval time.Duration
	CleanAge      time.Duration
	Permissions   os.FileMode
}

type FileError struct {
	Op   string
	Path string
	Err  error
}

func (e *FileError) Error() string {
	if e.Path != "" {
		return e.Op + " " + e.Path + ": " + e.Err.Error()
	}
	return e.Op + ": " + e.Err.Error()
}

func (e *FileError) Unwrap() error {
	return e.Err
}

type UploadOptions struct {
	AllowedExtensions []string
	MaxSize           int64
	AllowedMimeTypes  []string
	OverwriteExisting bool
	PreserveOriginal  bool
	ValidatePath      bool
}

type DownloadOptions struct {
	BufferSize int
	Compress   bool
}

type ListOptions struct {
	Recursive   bool
	Pattern     string
	IncludeDirs bool
	SortBy      string
	SortOrder   string
	Limit       int
	Offset      int
}

func DefaultStorageConfig() *StorageConfig {
	return &StorageConfig{
		BasePath:      "./storage",
		MaxFileSize:   100 * 1024 * 1024,
		AllowedTypes:  []string{"*"},
		TempDir:       "./storage/tmp",
		AutoClean:     true,
		CleanInterval: 24 * time.Hour,
		CleanAge:      7 * 24 * time.Hour,
		Permissions:   0755,
	}
}

func DefaultUploadOptions() *UploadOptions {
	return &UploadOptions{
		AllowedExtensions: []string{".html", ".txt", ".csv", ".pdf", ".jpg", ".png", ".zip"},
		MaxSize:           50 * 1024 * 1024,
		AllowedMimeTypes:  []string{"*"},
		OverwriteExisting: false,
		PreserveOriginal:  true,
		ValidatePath:      true,
	}
}

func DefaultListOptions() *ListOptions {
	return &ListOptions{
		Recursive:   false,
		Pattern:     "*",
		IncludeDirs: true,
		SortBy:      "name",
		SortOrder:   "asc",
		Limit:       1000,
		Offset:      0,
	}
}
