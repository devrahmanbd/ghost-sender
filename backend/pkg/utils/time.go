package utils

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrFileNotFound     = errors.New("file not found")
	ErrInvalidPath      = errors.New("invalid file path")
	ErrPathTraversal    = errors.New("path traversal detected")
	ErrInvalidExtension = errors.New("invalid file extension")
	ErrFileTooLarge     = errors.New("file too large")
	ErrDirectoryExists  = errors.New("directory already exists")
)

const (
	DefaultFilePermission = 0644
	DefaultDirPermission  = 0755
	MaxFileSize           = 100 * 1024 * 1024
)

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func DirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func IsFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func IsDir(path string) bool {
	return DirExists(path)
}

func FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func FileModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func ReadFile(path string) ([]byte, error) {
	if err := ValidatePath(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func ReadFileString(path string) (string, error) {
	data, err := ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func WriteFile(path string, data []byte) error {
	if err := ValidatePath(path); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return err
	}

	return os.WriteFile(path, data, DefaultFilePermission)
}

func WriteFileString(path string, content string) error {
	return WriteFile(path, []byte(content))
}

func AppendFile(path string, data []byte) error {
	if err := ValidatePath(path); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, DefaultFilePermission)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

func AppendFileString(path string, content string) error {
	return AppendFile(path, []byte(content))
}

func ReadLines(path string) ([]string, error) {
	if err := ValidatePath(path); err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func WriteLines(path string, lines []string) error {
	content := strings.Join(lines, "\n")
	return WriteFileString(path, content)
}

func CopyFile(src, dst string) error {
	if err := ValidatePath(src); err != nil {
		return err
	}
	if err := ValidatePath(dst); err != nil {
		return err
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}

func MoveFile(src, dst string) error {
	if err := ValidatePath(src); err != nil {
		return err
	}
	if err := ValidatePath(dst); err != nil {
		return err
	}

	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}

	err := os.Rename(src, dst)
	if err != nil {
		if err := CopyFile(src, dst); err != nil {
			return err
		}
		return os.Remove(src)
	}

	return nil
}

func DeleteFile(path string) error {
	if err := ValidatePath(path); err != nil {
		return err
	}
	return os.Remove(path)
}

func DeleteDir(path string) error {
	if err := ValidatePath(path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func CreateDir(path string) error {
	if err := ValidatePath(path); err != nil {
		return err
	}
	return os.MkdirAll(path, DefaultDirPermission)
}

func EnsureDir(path string) error {
	if DirExists(path) {
		return nil
	}
	return CreateDir(path)
}

func ListFiles(dir string) ([]string, error) {
	if err := ValidatePath(dir); err != nil {
		return nil, err
	}

	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}

func ListDirs(dir string) ([]string, error) {
	if err := ValidatePath(dir); err != nil {
		return nil, err
	}

	var dirs []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(dir, entry.Name()))
		}
	}

	return dirs, nil
}

func ListAll(dir string) ([]string, error) {
	if err := ValidatePath(dir); err != nil {
		return nil, err
	}

	var all []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		all = append(all, filepath.Join(dir, entry.Name()))
	}

	return all, nil
}

func WalkDir(root string, fn func(path string, info fs.FileInfo, err error) error) error {
	if err := ValidatePath(root); err != nil {
		return err
	}
	return filepath.Walk(root, fn)
}

func GetExtension(path string) string {
	ext := filepath.Ext(path)
	return strings.ToLower(ext)
}

func GetBasename(path string) string {
	return filepath.Base(path)
}

func GetFilename(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func GetDir(path string) string {
	return filepath.Dir(path)
}

func JoinPath(parts ...string) string {
	return filepath.Join(parts...)
}

func AbsPath(path string) (string, error) {
	return filepath.Abs(path)
}

func RelPath(basepath, targpath string) (string, error) {
	return filepath.Rel(basepath, targpath)
}

func ValidatePath(path string) error {
	if path == "" {
		return ErrInvalidPath
	}

	cleanPath := filepath.Clean(path)

	if strings.Contains(cleanPath, "..") {
		return ErrPathTraversal
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return err
	}

	if strings.Contains(absPath, "..") {
		return ErrPathTraversal
	}

	return nil
}

func SanitizeFilename(filename string) string {
	filename = strings.TrimSpace(filename)

	filename = strings.ReplaceAll(filename, "..", "")
	filename = strings.ReplaceAll(filename, "/", "")
	filename = strings.ReplaceAll(filename, "\\", "")
	filename = strings.ReplaceAll(filename, "\x00", "")

	invalidChars := []string{"<", ">", ":", "\"", "|", "?", "*"}
	for _, char := range invalidChars {
		filename = strings.ReplaceAll(filename, char, "_")
	}

	if len(filename) > 255 {
		ext := filepath.Ext(filename)
		name := strings.TrimSuffix(filename, ext)
		filename = name[:255-len(ext)] + ext
	}

	return filename
}

func ValidateExtension(filename string, allowedExts []string) error {
	ext := GetExtension(filename)
	ext = strings.TrimPrefix(ext, ".")

	for _, allowed := range allowedExts {
		allowed = strings.ToLower(strings.TrimPrefix(allowed, "."))
		if ext == allowed {
			return nil
		}
	}

	return ErrInvalidExtension
}

func DetectMimeType(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	if n > 0 {
		detected := detectMimeFromBytes(buffer[:n])
		if detected != "" {
			mimeType = detected
		}
	}

	return mimeType, nil
}

func detectMimeFromBytes(data []byte) string {
	if len(data) < 4 {
		return ""
	}

	if data[0] == 0x25 && data[1] == 0x50 && data[2] == 0x44 && data[3] == 0x46 {
		return "application/pdf"
	}

	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}

	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}

	if len(data) >= 12 {
		if string(data[8:12]) == "WEBP" {
			return "image/webp"
		}
	}

	if data[0] == 0x50 && data[1] == 0x4B {
		return "application/zip"
	}

	if len(data) >= 5 {
		if string(data[0:5]) == "<?xml" || string(data[0:5]) == "<html" {
			return "text/html"
		}
	}

	return ""
}

func CalculateFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func CalculateHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func CreateTempFile(pattern string) (*os.File, error) {
	return os.CreateTemp("", pattern)
}

func CreateTempDir(pattern string) (string, error) {
	return os.MkdirTemp("", pattern)
}

func CheckFileSize(path string, maxSize int64) error {
	size, err := FileSize(path)
	if err != nil {
		return err
	}

	if size > maxSize {
		return ErrFileTooLarge
	}

	return nil
}

func ExtractZip(zipPath, destDir string) error {
	if err := ValidatePath(zipPath); err != nil {
		return err
	}
	if err := ValidatePath(destDir); err != nil {
		return err
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := EnsureDir(destDir); err != nil {
		return err
	}

	for _, f := range r.File {
		if err := extractZipFile(f, destDir); err != nil {
			return err
		}
	}

	return nil
}

func extractZipFile(f *zip.File, destDir string) error {
	filePath := filepath.Join(destDir, f.Name)

	if err := ValidatePath(filePath); err != nil {
		return err
	}

	if !strings.HasPrefix(filePath, filepath.Clean(destDir)+string(os.PathSeparator)) {
		return ErrPathTraversal
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(filePath, DefaultDirPermission)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), DefaultDirPermission); err != nil {
		return err
	}

	destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer destFile.Close()

	zipFile, err := f.Open()
	if err != nil {
		return err
	}
	defer zipFile.Close()

	_, err = io.Copy(destFile, zipFile)
	return err
}

func CreateZip(sourceDir, zipPath string) error {
	if err := ValidatePath(sourceDir); err != nil {
		return err
	}
	if err := ValidatePath(zipPath); err != nil {
		return err
	}

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	return filepath.Walk(sourceDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}

		zipEntry, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(zipEntry, file)
		return err
	})
}

func ValidateZipStructure(zipPath string, requiredFiles []string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	foundFiles := make(map[string]bool)
	for _, f := range r.File {
		foundFiles[f.Name] = true
	}

	for _, required := range requiredFiles {
		if !foundFiles[required] {
			return fmt.Errorf("required file not found in zip: %s", required)
		}
	}

	return nil
}

func GetFilesByExtension(dir string, extensions []string) ([]string, error) {
	files, err := ListFiles(dir)
	if err != nil {
		return nil, err
	}

	var matched []string
	for _, file := range files {
		ext := GetExtension(file)
		for _, allowedExt := range extensions {
			allowedExt = strings.ToLower(strings.TrimPrefix(allowedExt, "."))
			if strings.TrimPrefix(ext, ".") == allowedExt {
				matched = append(matched, file)
				break
			}
		}
	}

	return matched, nil
}

func CountFiles(dir string) (int, error) {
	files, err := ListFiles(dir)
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

func DirSize(dir string) (int64, error) {
	var size int64

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

func CleanPath(p string) string {
	return filepath.Clean(p)
}

func MatchPattern(pattern, name string) (bool, error) {
	return path.Match(pattern, name)
}

func IsAbsolutePath(path string) bool {
	return filepath.IsAbs(path)
}

func TouchFile(path string) error {
	if FileExists(path) {
		currentTime := time.Now()
		return os.Chtimes(path, currentTime, currentTime)
	}
	return WriteFile(path, []byte{})
}

func ChmodFile(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

func GetFileInfo(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
