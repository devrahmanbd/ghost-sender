package handlers

import (
        "archive/zip"
        "context"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "os"
        "path/filepath"
        "strings"
        "time"
    "github.com/gorilla/mux"

        "email-campaign-system/internal/api/websocket"
        "email-campaign-system/internal/storage/files"
        "email-campaign-system/pkg/errors"
        "email-campaign-system/pkg/logger"
        "email-campaign-system/pkg/validator"
)

type AttachmentRefresher interface {
        RefreshTemplates() error
}

type FileHandler struct {
        fileStorage    files.Storage
        wsHub          *websocket.Hub
        logger         logger.Logger
        validator      *validator.Validator
        maxFileSize    int64
        allowedTypes   []string
        basePath       string
        attachmentMgr  AttachmentRefresher
}

func NewFileHandler(
        fileStorage files.Storage,
        wsHub *websocket.Hub,
        logger logger.Logger,
        validator *validator.Validator,
        maxFileSize int64,
        allowedTypes []string,
        basePath string,
        opts ...FileHandlerOption,
) *FileHandler {
        h := &FileHandler{
                fileStorage:  fileStorage,
                wsHub:        wsHub,
                logger:       logger,
                validator:    validator,
                maxFileSize:  maxFileSize,
                allowedTypes: allowedTypes,
                basePath:     basePath,
        }
        for _, o := range opts {
                o(h)
        }
        return h
}

type FileHandlerOption func(*FileHandler)

func WithAttachmentManager(mgr AttachmentRefresher) FileHandlerOption {
        return func(h *FileHandler) {
                h.attachmentMgr = mgr
        }
}

type FileInfo struct {
        Name      string    `json:"name"`
        Path      string    `json:"path"`
        Size      int64     `json:"size"`
        Type      string    `json:"type"`
        MimeType  string    `json:"mime_type"`
        IsDir     bool      `json:"is_dir"`
        CreatedAt time.Time `json:"created_at"`
        UpdatedAt time.Time `json:"updated_at"`
}

type UploadResponse struct {
        Success  bool   `json:"success"`
        Message  string `json:"message"`
        FileName string `json:"file_name"`
        FilePath string `json:"file_path"`
        FileSize int64  `json:"file_size"`
        FileType string `json:"file_type"`
}

type ZIPUploadResponse struct {
        Success       bool     `json:"success"`
        Message       string   `json:"message"`
        ExtractedPath string   `json:"extracted_path"`
        FilesCount    int      `json:"files_count"`
        Files         []string `json:"files"`
}

func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
                h.respondError(w, errors.BadRequest("File too large or invalid form data"))
                return
        }

        file, header, err := r.FormFile("file")
        if err != nil {
                h.respondError(w, errors.BadRequest("No file uploaded"))
                return
        }
        defer file.Close()

        category := r.FormValue("category")
        if category == "" {
                category = "general"
        }

        if !h.isValidCategory(category) {
                h.respondError(w, errors.BadRequest("Invalid file category"))
                return
        }

        if header.Size > h.maxFileSize {
                h.respondError(w, errors.BadRequest(fmt.Sprintf("File size exceeds maximum allowed size of %d bytes", h.maxFileSize)))
                return
        }

        ext := filepath.Ext(header.Filename)
        if !h.isAllowedFileType(ext) {
                h.respondError(w, errors.BadRequest("File type not allowed"))
                return
        }

        if err := h.validateFileName(header.Filename); err != nil {
                h.respondError(w, errors.BadRequest("Invalid file name: "+err.Error()))
                return
        }

        filePath := filepath.Join(category, header.Filename)

        if err := h.fileStorage.WriteReader(ctx, filePath, file); err != nil {
                h.logger.Error("Failed to save file", logger.String("filename", header.Filename), logger.Error(err))
                h.respondError(w, errors.Internal("Failed to save file"))
                return
        }

        h.logger.Info("File uploaded successfully", logger.String("filename", header.Filename), logger.String("path", filePath), logger.Int64("size", header.Size))

        dataJSON, _ := json.Marshal(map[string]interface{}{
                "filename": header.Filename,
                "category": category,
                "size":     header.Size,
        })

        h.wsHub.Broadcast(&websocket.Message{
                Type: "file_uploaded",
                Data: json.RawMessage(dataJSON),
        })

        if (category == "templates" || category == "attachments") && h.attachmentMgr != nil {
                if err := h.attachmentMgr.RefreshTemplates(); err != nil {
                        h.logger.Warn("Failed to refresh attachment templates after upload", logger.Error(err))
                }
        }

        response := UploadResponse{
                Success:  true,
                Message:  "File uploaded successfully",
                FileName: header.Filename,
                FilePath: filePath,
                FileSize: header.Size,
                FileType: ext,
        }

        h.respondJSON(w, http.StatusOK, response)
}

func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        category := vars["category"]
        filename := vars["filename"]

        if !h.isValidCategory(category) {
                h.respondError(w, errors.BadRequest("Invalid file category"))
                return
        }

        if err := h.validateFileName(filename); err != nil {
                h.respondError(w, errors.BadRequest("Invalid file name"))
                return
        }

        filePath := filepath.Join(category, filename)

        exists, err := h.fileStorage.Exists(ctx, filePath)
        if err != nil {
                h.logger.Error("Failed to check file existence", logger.String("path", filePath), logger.Error(err))
                h.respondError(w, errors.Internal("Failed to check file"))
                return
        }

        if !exists {
                h.respondError(w, errors.NotFound("file_not_found", "File not found"))
                return
        }

        stream, err := h.fileStorage.ReadStream(ctx, filePath)
        if err != nil {
                h.logger.Error("Failed to open file", logger.String("path", filePath), logger.Error(err))
                h.respondError(w, errors.Internal("Failed to open file"))
                return
        }
        defer stream.Close()

        fileInfo, err := h.fileStorage.GetInfo(ctx, filePath)
        if err != nil {
                h.logger.Error("Failed to get file stats", logger.String("path", filePath), logger.Error(err))
                h.respondError(w, errors.Internal("Failed to get file info"))
                return
        }

        contentType := h.getContentType(filename)

        w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
        w.Header().Set("Content-Type", contentType)
        w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size))

        if _, err := io.Copy(w, stream); err != nil {
                h.logger.Error("Failed to send file", logger.String("path", filePath), logger.Error(err))
                return
        }

        h.logger.Info("File downloaded", logger.String("filename", filename), logger.String("category", category))
}

func (h *FileHandler) UploadZIP(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()

        if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
                h.respondError(w, errors.BadRequest("File too large or invalid form data"))
                return
        }

        file, header, err := r.FormFile("file")
        if err != nil {
                h.respondError(w, errors.BadRequest("No file uploaded"))
                return
        }
        defer file.Close()

        if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
                h.respondError(w, errors.BadRequest("Only ZIP files are allowed"))
                return
        }

        if header.Size > h.maxFileSize {
                h.respondError(w, errors.BadRequest(fmt.Sprintf("File size exceeds maximum allowed size of %d bytes", h.maxFileSize)))
                return
        }

        tempFile, err := os.CreateTemp("", "campaign-*.zip")
        if err != nil {
                h.logger.Error("Failed to create temp file", logger.Error(err))
                h.respondError(w, errors.Internal("Failed to process ZIP file"))
                return
        }
        defer os.Remove(tempFile.Name())
        defer tempFile.Close()

        if _, err := io.Copy(tempFile, file); err != nil {
                h.logger.Error("Failed to copy ZIP file", logger.Error(err))
                h.respondError(w, errors.Internal("Failed to process ZIP file"))
                return
        }

        extractPath := filepath.Join("campaigns", strings.TrimSuffix(header.Filename, ".zip"))

        if err := h.fileStorage.CreateDir(ctx, extractPath); err != nil {
                h.logger.Error("Failed to create extraction directory", logger.Error(err))
                h.respondError(w, errors.Internal("Failed to extract ZIP file"))
                return
        }

        files, err := h.extractZIP(ctx, tempFile.Name(), extractPath)
        if err != nil {
                h.logger.Error("Failed to extract ZIP file", logger.Error(err))
                h.fileStorage.DeleteDir(ctx, extractPath)
                h.respondError(w, errors.BadRequest("Failed to extract ZIP file: "+err.Error()))
                return
        }

        h.logger.Info("ZIP file extracted successfully", logger.String("filename", header.Filename), logger.Int("files_count", len(files)))

        dataJSON, _ := json.Marshal(map[string]interface{}{
                "filename":    header.Filename,
                "files_count": len(files),
                "path":        extractPath,
        })

        h.wsHub.Broadcast(&websocket.Message{
                Type: "zip_extracted",
                Data: json.RawMessage(dataJSON),
        })

        response := ZIPUploadResponse{
                Success:       true,
                Message:       "ZIP file extracted successfully",
                ExtractedPath: extractPath,
                FilesCount:    len(files),
                Files:         files,
        }

        h.respondJSON(w, http.StatusOK, response)
}

func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        category := vars["category"]

        if !h.isValidCategory(category) {
                h.respondError(w, errors.BadRequest("Invalid file category"))
                return
        }

        dirPath := category

        exists, err := h.fileStorage.Exists(ctx, dirPath)
        if err != nil || !exists {
                h.respondJSON(w, http.StatusOK, map[string]interface{}{
                        "files": []FileInfo{},
                        "total": 0,
                })
                return
        }

        fileInfos, err := h.fileStorage.List(ctx, dirPath)
        if err != nil {
                h.logger.Error("Failed to read directory", logger.String("path", dirPath), logger.Error(err))
                h.respondError(w, errors.Internal("Failed to list files"))
                return
        }

        files := make([]FileInfo, 0, len(fileInfos))
        for _, info := range fileInfos {
                fileInfo := FileInfo{
                        Name:      info.Name,
                        Path:      filepath.Join(category, info.Name),
                        Size:      info.Size,
                        Type:      filepath.Ext(info.Name),
                        MimeType:  info.MimeType,
                        IsDir:     info.IsDir,
                        CreatedAt: info.ModTime,
                        UpdatedAt: info.ModTime,
                }
                files = append(files, fileInfo)
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "files": files,
                "total": len(files),
        })
}

func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        category := vars["category"]
        filename := vars["filename"]

        if !h.isValidCategory(category) {
                h.respondError(w, errors.BadRequest("Invalid file category"))
                return
        }

        if err := h.validateFileName(filename); err != nil {
                h.respondError(w, errors.BadRequest("Invalid file name"))
                return
        }

        filePath := filepath.Join(category, filename)

        exists, err := h.fileStorage.Exists(ctx, filePath)
        if err != nil {
                h.logger.Error("Failed to check file existence", logger.String("path", filePath), logger.Error(err))
                h.respondError(w, errors.Internal("Failed to check file"))
                return
        }

        if !exists {
                h.respondError(w, errors.NotFound("file_not_found", "File not found"))
                return
        }

        if err := h.fileStorage.Delete(ctx, filePath); err != nil {
                h.logger.Error("Failed to delete file", logger.String("path", filePath), logger.Error(err))
                h.respondError(w, errors.Internal("Failed to delete file"))
                return
        }

        h.logger.Info("File deleted successfully", logger.String("filename", filename), logger.String("category", category))

        dataJSON, _ := json.Marshal(map[string]interface{}{
                "filename": filename,
                "category": category,
        })

        h.wsHub.Broadcast(&websocket.Message{
                Type: "file_deleted",
                Data: json.RawMessage(dataJSON),
        })

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "success": true,
                "message": "File deleted successfully",
        })
}

func (h *FileHandler) GetFileInfo(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        vars := mux.Vars(r)
        category := vars["category"]
        filename := vars["filename"]

        if !h.isValidCategory(category) {
                h.respondError(w, errors.BadRequest("Invalid file category"))
                return
        }

        if err := h.validateFileName(filename); err != nil {
                h.respondError(w, errors.BadRequest("Invalid file name"))
                return
        }

        filePath := filepath.Join(category, filename)

        exists, err := h.fileStorage.Exists(ctx, filePath)
        if err != nil {
                h.logger.Error("Failed to check file existence", logger.String("path", filePath), logger.Error(err))
                h.respondError(w, errors.Internal("Failed to check file"))
                return
        }

        if !exists {
                h.respondError(w, errors.NotFound("file_not_found", "File not found"))
                return
        }

        info, err := h.fileStorage.GetInfo(ctx, filePath)
        if err != nil {
                h.logger.Error("Failed to get file stats", logger.String("path", filePath), logger.Error(err))
                h.respondError(w, errors.Internal("Failed to get file info"))
                return
        }

        fileInfo := FileInfo{
                Name:      filename,
                Path:      filepath.Join(category, filename),
                Size:      info.Size,
                Type:      filepath.Ext(filename),
                MimeType:  info.MimeType,
                IsDir:     info.IsDir,
                CreatedAt: info.ModTime,
                UpdatedAt: info.ModTime,
        }

        h.respondJSON(w, http.StatusOK, fileInfo)
}

func (h *FileHandler) extractZIP(ctx context.Context, zipPath, destPath string) ([]string, error) {
        reader, err := zip.OpenReader(zipPath)
        if err != nil {
                return nil, err
        }
        defer reader.Close()

        var extractedFiles []string

        for _, file := range reader.File {
                if err := h.validateZIPEntry(file.Name); err != nil {
                        return nil, err
                }

                relPath := file.Name
                filePath := filepath.Join(destPath, relPath)

                if !strings.HasPrefix(filepath.Clean(filePath), filepath.Clean(destPath)) {
                        return nil, fmt.Errorf("invalid file path: %s", file.Name)
                }

                if file.FileInfo().IsDir() {
                        h.fileStorage.CreateDir(ctx, filePath)
                        continue
                }

                rc, err := file.Open()
                if err != nil {
                        return nil, err
                }

                if err := h.fileStorage.WriteReader(ctx, filePath, rc); err != nil {
                        rc.Close()
                        return nil, err
                }
                rc.Close()

                extractedFiles = append(extractedFiles, file.Name)
        }

        return extractedFiles, nil
}

func (h *FileHandler) validateFileName(filename string) error {
        if strings.Contains(filename, "..") {
                return fmt.Errorf("path traversal detected")
        }

        if strings.ContainsAny(filename, `<>:"|?*\\`) {
                return fmt.Errorf("invalid characters in filename")
        }

        if len(filename) > 255 {
                return fmt.Errorf("filename too long")
        }

        return nil
}

func (h *FileHandler) validateZIPEntry(entryName string) error {
        if strings.Contains(entryName, "..") {
                return fmt.Errorf("path traversal detected in ZIP entry: %s", entryName)
        }

        if strings.HasPrefix(entryName, "/") {
                return fmt.Errorf("absolute path in ZIP entry: %s", entryName)
        }

        return nil
}

func (h *FileHandler) isValidCategory(category string) bool {
        validCategories := []string{"templates", "attachments", "configs", "campaigns", "logs", "exports", "general"}
        for _, valid := range validCategories {
                if category == valid {
                        return true
                }
        }
        return false
}

func (h *FileHandler) isAllowedFileType(ext string) bool {
        if len(h.allowedTypes) == 0 {
                return true
        }

        ext = strings.ToLower(ext)
        for _, allowed := range h.allowedTypes {
                if ext == strings.ToLower(allowed) {
                        return true
                }
        }
        return false
}

func (h *FileHandler) getContentType(filename string) string {
        ext := strings.ToLower(filepath.Ext(filename))
        contentTypes := map[string]string{
                ".html": "text/html",
                ".css":  "text/css",
                ".js":   "application/javascript",
                ".json": "application/json",
                ".xml":  "application/xml",
                ".txt":  "text/plain",
                ".csv":  "text/csv",
                ".pdf":  "application/pdf",
                ".zip":  "application/zip",
                ".jpg":  "image/jpeg",
                ".jpeg": "image/jpeg",
                ".png":  "image/png",
                ".gif":  "image/gif",
                ".svg":  "image/svg+xml",
                ".webp": "image/webp",
        }

        if contentType, ok := contentTypes[ext]; ok {
                return contentType
        }

        return "application/octet-stream"
}

func (h *FileHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(status)
        json.NewEncoder(w).Encode(data)
}

func (h *FileHandler) respondError(w http.ResponseWriter, err error) {
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
// ExtractZip extracts a ZIP file
func (h *FileHandler) ExtractZip(w http.ResponseWriter, r *http.Request) {

    var req struct {
        ZipPath     string `json:"zip_path" validate:"required"`
        ExtractPath string `json:"extract_path" validate:"required"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, errors.BadRequest("Invalid request body"))
        return
    }

    if err := h.validator.Validate(req); err != nil {
        h.respondError(w, errors.ValidationError("validation", []string{err.Error()})) // ✅ Fixed: array of strings
        return
    }

    // Security: Validate paths to prevent directory traversal
    if strings.Contains(req.ZipPath, "..") || strings.Contains(req.ExtractPath, "..") {
        h.respondError(w, errors.BadRequest("Invalid path: directory traversal not allowed"))
        return
    }

    // Open ZIP file
    zipReader, err := zip.OpenReader(req.ZipPath)
    if err != nil {
        h.logger.Error("Failed to open ZIP file",
            logger.String("zippath", req.ZipPath),
            logger.Error(err),
        )
        h.respondError(w, errors.BadRequest("Invalid ZIP file"))
        return
    }
    defer zipReader.Close()

    // Create extract directory
    if err := os.MkdirAll(req.ExtractPath, 0755); err != nil {
        h.logger.Error("Failed to create extract directory", logger.Error(err))
        h.respondError(w, errors.Internal("Failed to create extract directory"))
        return
    }

    var extractedFiles []string

    // Extract files
    for _, file := range zipReader.File {
        // Security: Prevent zip slip vulnerability
        filePath := filepath.Join(req.ExtractPath, file.Name)
        if !strings.HasPrefix(filePath, filepath.Clean(req.ExtractPath)+string(os.PathSeparator)) {
            h.logger.Warn("Skipping file outside extract path", logger.String("file", file.Name))
            continue
        }

        if file.FileInfo().IsDir() {
            if err := os.MkdirAll(filePath, file.Mode()); err != nil {
                h.logger.Error("Failed to create directory", logger.Error(err))
            }
            continue
        }

        // Create parent directory
        if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
            h.logger.Error("Failed to create directory", logger.Error(err))
            continue
        }

        // Extract file
        outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
        if err != nil {
            h.logger.Error("Failed to create file", logger.Error(err))
            continue
        }

        rc, err := file.Open()
        if err != nil {
            outFile.Close()
            h.logger.Error("Failed to open file in archive", logger.Error(err))
            continue
        }

        _, err = io.Copy(outFile, rc)
        outFile.Close()
        rc.Close()

        if err != nil {
            h.logger.Error("Failed to extract file", logger.Error(err))
            continue
        }

        extractedFiles = append(extractedFiles, file.Name)
    }

    h.logger.Info("ZIP extracted",
        logger.String("zippath", req.ZipPath),
        logger.Int("files", len(extractedFiles)),
    )

    h.respondJSON(w, http.StatusOK, map[string]interface{}{
        "success":      true,
        "files":        extractedFiles,
        "total":        len(extractedFiles),
        "extract_path": req.ExtractPath,
    })
}
func (h *FileHandler) CreateZip(w http.ResponseWriter, r *http.Request) {

    var req struct {
        Files   []string `json:"files" validate:"required,min=1"`
        ZipName string   `json:"zip_name" validate:"required"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, errors.BadRequest("Invalid request body"))
        return
    }

    if err := h.validator.Validate(req); err != nil {
        h.respondError(w, errors.ValidationError("validation", []string{err.Error()})) // ✅ Fixed: array of strings
        return
    }

    // Security: Validate zip name
    if strings.Contains(req.ZipName, "..") || strings.Contains(req.ZipName, "/") {
        h.respondError(w, errors.BadRequest("Invalid zip name"))
        return
    }

    // Ensure .zip extension
    if !strings.HasSuffix(req.ZipName, ".zip") {
        req.ZipName += ".zip"
    }

    zipPath := filepath.Join("./storage/archives", req.ZipName)

    // Create archives directory
    if err := os.MkdirAll("./storage/archives", 0755); err != nil {
        h.logger.Error("Failed to create archives directory", logger.Error(err))
        h.respondError(w, errors.Internal("Failed to create archives directory"))
        return
    }

    // Create ZIP file
    zipFile, err := os.Create(zipPath)
    if err != nil {
        h.logger.Error("Failed to create ZIP file", logger.Error(err))
        h.respondError(w, errors.Internal("Failed to create ZIP file"))
        return
    }
    defer zipFile.Close()

    zipWriter := zip.NewWriter(zipFile)
    defer zipWriter.Close()

    successCount := 0
    failedFiles := []string{}

    // Add files to ZIP
    for _, filePath := range req.Files {
        if err := h.addFileToZip(zipWriter, filePath); err != nil {
            h.logger.Error("Failed to add file to ZIP",
                logger.String("file", filePath),
                logger.Error(err),
            )
            failedFiles = append(failedFiles, filePath)
            continue
        }
        successCount++
    }

    h.logger.Info("ZIP created",
        logger.String("zippath", zipPath),
        logger.Int("total_files", len(req.Files)),
        logger.Int("successful", successCount),
        logger.Int("failed", len(failedFiles)),
    )

    response := map[string]interface{}{
        "success":  true,
        "zip_path": zipPath,
        "total":    len(req.Files),
        "added":    successCount,
    }

    if len(failedFiles) > 0 {
        response["failed"] = len(failedFiles)
        response["failed_files"] = failedFiles
    }

    h.respondJSON(w, http.StatusOK, response)
}

// addFileToZip is a helper function to add a file to ZIP archive
func (h *FileHandler) addFileToZip(zipWriter *zip.Writer, filename string) error {
    // Security: Validate file path
    if strings.Contains(filename, "..") {
        return errors.BadRequest("Invalid file path: directory traversal not allowed")
    }

    file, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    info, err := file.Stat()
    if err != nil {
        return err
    }

    // Create ZIP header
    header, err := zip.FileInfoHeader(info)
    if err != nil {
        return err
    }

    // Use base name to avoid directory structure in ZIP
    header.Name = filepath.Base(filename)
    header.Method = zip.Deflate

    writer, err := zipWriter.CreateHeader(header)
    if err != nil {
        return err
    }

    // Copy file content to ZIP
    _, err = io.Copy(writer, file)
    return err
}
