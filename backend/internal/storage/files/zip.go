package files

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ZipHandler struct {
	storage       Storage
	maxSize       int64
	maxExtracted  int64
	maxFiles      int
	allowedExts   []string
	validatePaths bool
}

type ZipOptions struct {
	CompressionLevel int
	IncludeHidden    bool
	PreservePaths    bool
	Overwrite        bool
	FileFilter       func(string) bool
	ProgressCallback func(current, total int64)
}

type ZipInfo struct {
	TotalFiles       int
	TotalSize        int64
	CompressedSize   int64
	Files            []*ZipFileInfo
	CompressionRatio float64
}

type ZipFileInfo struct {
	Name             string
	Size             int64
	CompressedSize   int64
	Modified         time.Time
	IsDir            bool
	CompressionRatio float64
}

func NewZipHandler(storage Storage, maxSize, maxExtracted int64, maxFiles int) *ZipHandler {
	return &ZipHandler{
		storage:       storage,
		maxSize:       maxSize,
		maxExtracted:  maxExtracted,
		maxFiles:      maxFiles,
		validatePaths: true,
		allowedExts:   []string{".html", ".txt", ".csv", ".json", ".yaml", ".yml", ".ini", ".conf"},
	}
}

func (z *ZipHandler) Create(ctx context.Context, sourcePath, zipPath string, opts *ZipOptions) error {
	if opts == nil {
		opts = DefaultZipOptions()
	}

	if err := z.validatePath(sourcePath); err != nil {
		return &FileError{Op: "zip_create", Path: sourcePath, Err: err}
	}

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return &FileError{Op: "zip_create", Path: zipPath, Err: err}
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	var totalFiles int64
	var processedFiles int64

	err = filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalFiles++
		}
		return nil
	})
	if err != nil {
		return err
	}

	return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !opts.IncludeHidden && strings.HasPrefix(filepath.Base(path), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if opts.FileFilter != nil && !opts.FileFilter(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if opts.PreservePaths {
			header.Name = relPath
		} else {
			header.Name = filepath.Base(path)
		}

		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		if err != nil {
			return err
		}

		processedFiles++
		if opts.ProgressCallback != nil {
			opts.ProgressCallback(processedFiles, totalFiles)
		}

		return nil
	})
}

func (z *ZipHandler) Extract(ctx context.Context, zipPath, destPath string, opts *ZipOptions) error {
	if opts == nil {
		opts = DefaultZipOptions()
	}

	if err := z.validatePath(destPath); err != nil {
		return &FileError{Op: "zip_extract", Path: destPath, Err: err}
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return &FileError{Op: "zip_extract", Path: zipPath, Err: err}
	}
	defer reader.Close()

	if len(reader.File) > z.maxFiles {
		return &FileError{Op: "zip_extract", Path: zipPath, Err: fmt.Errorf("too many files in archive")}
	}

	var totalSize int64
	for _, file := range reader.File {
		totalSize += int64(file.UncompressedSize64)
	}

	if z.maxExtracted > 0 && totalSize > z.maxExtracted {
		return &FileError{Op: "zip_extract", Path: zipPath, Err: fmt.Errorf("extracted size would exceed limit")}
	}

	totalFiles := int64(len(reader.File))
	var processedFiles int64

	for _, file := range reader.File {
		if err := z.extractFile(file, destPath, opts); err != nil {
			return err
		}

		processedFiles++
		if opts.ProgressCallback != nil {
			opts.ProgressCallback(processedFiles, totalFiles)
		}
	}

	return nil
}

func (z *ZipHandler) extractFile(file *zip.File, destPath string, opts *ZipOptions) error {
	if err := z.validateZipPath(file.Name); err != nil {
		return &FileError{Op: "zip_extract", Path: file.Name, Err: err}
	}

	if !opts.IncludeHidden && strings.HasPrefix(filepath.Base(file.Name), ".") {
		return nil
	}

	if opts.FileFilter != nil && !opts.FileFilter(file.Name) {
		return nil
	}

	targetPath := filepath.Join(destPath, file.Name)

	if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destPath)) {
		return &FileError{Op: "zip_extract", Path: file.Name, Err: fmt.Errorf("path traversal detected")}
	}

	if file.FileInfo().IsDir() {
		return os.MkdirAll(targetPath, 0755)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}

	if !opts.Overwrite {
		if _, err := os.Stat(targetPath); err == nil {
			return nil
		}
	}

	outFile, err := os.Create(targetPath)
	if err != nil {
		return &FileError{Op: "zip_extract", Path: targetPath, Err: err}
	}
	defer outFile.Close()

	rc, err := file.Open()
	if err != nil {
		return &FileError{Op: "zip_extract", Path: file.Name, Err: err}
	}
	defer rc.Close()

	_, err = io.Copy(outFile, rc)
	if err != nil {
		os.Remove(targetPath)
		return &FileError{Op: "zip_extract", Path: targetPath, Err: err}
	}

	return nil
}

func (z *ZipHandler) List(ctx context.Context, zipPath string) (*ZipInfo, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, &FileError{Op: "zip_list", Path: zipPath, Err: err}
	}
	defer reader.Close()

	info := &ZipInfo{
		Files: make([]*ZipFileInfo, 0, len(reader.File)),
	}

	for _, file := range reader.File {
		fileInfo := &ZipFileInfo{
			Name:           file.Name,
			Size:           int64(file.UncompressedSize64),
			CompressedSize: int64(file.CompressedSize64),
			Modified:       file.Modified,
			IsDir:          file.FileInfo().IsDir(),
		}

		if fileInfo.Size > 0 {
			fileInfo.CompressionRatio = float64(fileInfo.CompressedSize) / float64(fileInfo.Size)
		}

		info.Files = append(info.Files, fileInfo)
		info.TotalFiles++
		info.TotalSize += fileInfo.Size
		info.CompressedSize += fileInfo.CompressedSize
	}

	if info.TotalSize > 0 {
		info.CompressionRatio = float64(info.CompressedSize) / float64(info.TotalSize)
	}

	return info, nil
}

func (z *ZipHandler) ReadFile(ctx context.Context, zipPath, filePath string) ([]byte, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, &FileError{Op: "zip_read", Path: zipPath, Err: err}
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == filePath {
			rc, err := file.Open()
			if err != nil {
				return nil, &FileError{Op: "zip_read", Path: filePath, Err: err}
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, &FileError{Op: "zip_read", Path: filePath, Err: err}
			}

			return data, nil
		}
	}

	return nil, &FileError{Op: "zip_read", Path: filePath, Err: fmt.Errorf("file not found in archive")}
}

func (z *ZipHandler) Validate(ctx context.Context, zipPath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return &FileError{Op: "zip_validate", Path: zipPath, Err: err}
	}
	defer reader.Close()

	if len(reader.File) > z.maxFiles {
		return fmt.Errorf("archive contains too many files")
	}

	var totalSize int64
	for _, file := range reader.File {
		if err := z.validateZipPath(file.Name); err != nil {
			return err
		}

		totalSize += int64(file.UncompressedSize64)
		if z.maxExtracted > 0 && totalSize > z.maxExtracted {
			return fmt.Errorf("archive size exceeds limit")
		}

		if z.maxSize > 0 && int64(file.UncompressedSize64) > z.maxSize {
			return fmt.Errorf("file size exceeds limit: %s", file.Name)
		}

		if len(z.allowedExts) > 0 {
			ext := filepath.Ext(file.Name)
			allowed := false
			for _, allowedExt := range z.allowedExts {
				if ext == allowedExt {
					allowed = true
					break
				}
			}
			if !allowed && !file.FileInfo().IsDir() {
				return fmt.Errorf("file extension not allowed: %s", file.Name)
			}
		}
	}

	return nil
}

func (z *ZipHandler) ExtractFile(ctx context.Context, zipPath, filePath, destPath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return &FileError{Op: "zip_extract_file", Path: zipPath, Err: err}
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == filePath {
			return z.extractFile(file, destPath, DefaultZipOptions())
		}
	}

	return &FileError{Op: "zip_extract_file", Path: filePath, Err: fmt.Errorf("file not found in archive")}
}

func (z *ZipHandler) AddFile(ctx context.Context, zipPath, filePath, archivePath string) error {
	tempPath := zipPath + ".tmp"

	outFile, err := os.Create(tempPath)
	if err != nil {
		return &FileError{Op: "zip_add", Path: tempPath, Err: err}
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	if _, err := os.Stat(zipPath); err == nil {
		reader, err := zip.OpenReader(zipPath)
		if err != nil {
			return &FileError{Op: "zip_add", Path: zipPath, Err: err}
		}
		defer reader.Close()

		for _, file := range reader.File {
			if file.Name == archivePath {
				continue
			}

			writer, err := zipWriter.CreateHeader(&file.FileHeader)
			if err != nil {
				return err
			}

			rc, err := file.Open()
			if err != nil {
				return err
			}

			_, err = io.Copy(writer, rc)
			rc.Close()
			if err != nil {
				return err
			}
		}
	}

	fileToAdd, err := os.Open(filePath)
	if err != nil {
		return &FileError{Op: "zip_add", Path: filePath, Err: err}
	}
	defer fileToAdd.Close()

	info, err := fileToAdd.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	header.Name = archivePath
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	if _, err := io.Copy(writer, fileToAdd); err != nil {
		return err
	}

	zipWriter.Close()
	outFile.Close()

	if err := os.Rename(tempPath, zipPath); err != nil {
		os.Remove(tempPath)
		return &FileError{Op: "zip_add", Path: zipPath, Err: err}
	}

	return nil
}

func (z *ZipHandler) validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path contains invalid elements")
	}

	return nil
}

func (z *ZipHandler) validateZipPath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	cleanPath := filepath.Clean(path)

	if filepath.IsAbs(cleanPath) {
		return fmt.Errorf("absolute paths not allowed")
	}

	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path traversal detected")
	}

	if strings.HasPrefix(cleanPath, "/") || strings.HasPrefix(cleanPath, "\\") {
		return fmt.Errorf("path cannot start with separator")
	}

	return nil
}

func DefaultZipOptions() *ZipOptions {
	return &ZipOptions{
		CompressionLevel: int(zip.Deflate),  
		IncludeHidden:    false,
		PreservePaths:    true,
		Overwrite:        false,
	}
}

func ExtractZipToStorage(ctx context.Context, zipPath string, storage Storage, destPath string, opts *ZipOptions) error {
	if opts == nil {
		opts = DefaultZipOptions()
	}

	tempDir, err := os.MkdirTemp("", "zip_extract_*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	handler := NewZipHandler(storage, 100*1024*1024, 500*1024*1024, 10000)
	if err := handler.Extract(ctx, zipPath, tempDir, opts); err != nil {
		return err
	}

	return filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(tempDir, path)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		storagePath := filepath.Join(destPath, relPath)
		return storage.Write(ctx, storagePath, data)
	})
}

func CreateZipFromStorage(ctx context.Context, storage Storage, sourcePath string, zipPath string, opts *ZipOptions) error {
	if opts == nil {
		opts = DefaultZipOptions()
	}

	tempDir, err := os.MkdirTemp("", "zip_create_*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	files, err := storage.List(ctx, sourcePath)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir {
			continue
		}

		data, err := storage.Read(ctx, file.Path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(tempDir, file.Name)
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return err
		}
	}

	handler := NewZipHandler(storage, 100*1024*1024, 500*1024*1024, 10000)
	return handler.Create(ctx, tempDir, zipPath, opts)
}
