package recipient

import (
    "bufio"
    "context"
    "encoding/csv"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "time"

    "email-campaign-system/internal/models"
    "email-campaign-system/pkg/logger"
)

var (
    ErrInvalidFileFormat   = errors.New("invalid file format")
    ErrFileNotFound        = errors.New("file not found")
    ErrEmptyFile           = errors.New("file is empty")
    ErrInvalidCSVStructure = errors.New("invalid CSV structure")
    ErrNoValidEmails       = errors.New("no valid emails found")
)

type Importer struct {
    validator *Validator
    logger    logger.Logger
    config    *ImportConfig
}

type ImportConfig struct {
    MaxFileSize      int64
    BatchSize        int
    EnableValidation bool
    SkipInvalid      bool
    TrimSpaces       bool
    LowercaseEmails  bool
    CSVDelimiter     rune
    CSVHasHeader     bool
    EmailColumn      int
    NameColumn       int
    CustomColumns    map[string]int
}

type ImportResult struct {
    TotalLines     int
    ValidEmails    int
    InvalidEmails  int
    DuplicateCount int
    SkippedLines   int
    Errors         []ImportError
    Recipients     []*models.Recipient
    Duration       time.Duration
    StartedAt      time.Time
    CompletedAt    time.Time
}

type ImportError struct {
    Line    int
    Email   string
    Reason  string
    Details string
}

type FileFormat string

const (
    FormatCSV     FileFormat = "csv"
    FormatTXT     FileFormat = "txt"
    FormatUnknown FileFormat = "unknown"
)

func NewImporter(validator *Validator, logger logger.Logger) *Importer {
    return &Importer{
        validator: validator,
        logger:    logger,
        config:    DefaultImportConfig(),
    }
}

func DefaultImportConfig() *ImportConfig {
    return &ImportConfig{
        MaxFileSize:      100 * 1024 * 1024,
        BatchSize:        1000,
        EnableValidation: true,
        SkipInvalid:      true,
        TrimSpaces:       true,
        LowercaseEmails:  true,
        CSVDelimiter:     ',',
        CSVHasHeader:     true,
        EmailColumn:      0,
        NameColumn:       1,
        CustomColumns:    make(map[string]int),
    }
}

func (imp *Importer) ImportFromFile(ctx context.Context, filePath string) ([]*models.Recipient, error) {
    startTime := time.Now()

    if err := imp.validateFile(filePath); err != nil {
        return nil, err
    }

    format := imp.detectFormat(filePath)
    if format == FormatUnknown {
        return nil, ErrInvalidFileFormat
    }

    imp.logger.Info(fmt.Sprintf("importing recipients: file=%s, format=%s", filePath, format))

    var recipients []*models.Recipient
    var err error

    switch format {
    case FormatCSV:
        recipients, err = imp.ImportFromCSV(ctx, filePath)
    case FormatTXT:
        recipients, err = imp.ImportFromTXT(ctx, filePath)
    default:
        return nil, ErrInvalidFileFormat
    }

    if err != nil {
        return nil, err
    }

    duration := time.Since(startTime)
    imp.logger.Info(fmt.Sprintf("import completed: file=%s, count=%d, duration=%v", 
        filePath, len(recipients), duration))

    return recipients, nil
}

func (imp *Importer) ImportFromCSV(ctx context.Context, filePath string) ([]*models.Recipient, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    reader := csv.NewReader(file)
    reader.Comma = imp.config.CSVDelimiter
    reader.TrimLeadingSpace = imp.config.TrimSpaces
    reader.FieldsPerRecord = -1

    var recipients []*models.Recipient
    lineNum := 0

    for {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        default:
        }

        record, err := reader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            imp.logger.Warn(fmt.Sprintf("csv read error: line=%d, error=%v", lineNum, err))
            continue
        }

        lineNum++

        if imp.config.CSVHasHeader && lineNum == 1 {
            continue
        }

        if len(record) == 0 {
            continue
        }

        recipient, err := imp.parseCSVRecord(record, lineNum)
        if err != nil {
            if imp.config.SkipInvalid {
                imp.logger.Debug(fmt.Sprintf("invalid record skipped: line=%d, error=%v", lineNum, err))
                continue
            }
            return nil, err
        }

        if recipient != nil {
            recipients = append(recipients, recipient)
        }
    }

    if len(recipients) == 0 {
        return nil, ErrNoValidEmails
    }

    return recipients, nil
}

func (imp *Importer) ImportFromTXT(ctx context.Context, filePath string) ([]*models.Recipient, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

    var recipients []*models.Recipient
    lineNum := 0

    for scanner.Scan() {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        default:
        }

        lineNum++
        line := scanner.Text()

        if imp.config.TrimSpaces {
            line = strings.TrimSpace(line)
        }

        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }

        recipient, err := imp.parseTXTLine(line, lineNum)
        if err != nil {
            if imp.config.SkipInvalid {
                imp.logger.Debug(fmt.Sprintf("invalid line skipped: line=%d, error=%v", lineNum, err))
                continue
            }
            return nil, err
        }

        if recipient != nil {
            recipients = append(recipients, recipient)
        }
    }

    if err := scanner.Err(); err != nil {
        return nil, err
    }

    if len(recipients) == 0 {
        return nil, ErrNoValidEmails
    }

    return recipients, nil
}

func (imp *Importer) ImportFromReader(ctx context.Context, reader io.Reader, format FileFormat) ([]*models.Recipient, error) {
    switch format {
    case FormatCSV:
        return imp.importCSVFromReader(ctx, reader)
    case FormatTXT:
        return imp.importTXTFromReader(ctx, reader)
    default:
        return nil, ErrInvalidFileFormat
    }
}

func (imp *Importer) importCSVFromReader(ctx context.Context, reader io.Reader) ([]*models.Recipient, error) {
    csvReader := csv.NewReader(reader)
    csvReader.Comma = imp.config.CSVDelimiter
    csvReader.TrimLeadingSpace = imp.config.TrimSpaces
    csvReader.FieldsPerRecord = -1

    var recipients []*models.Recipient
    lineNum := 0

    for {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        default:
        }

        record, err := csvReader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            continue
        }

        lineNum++

        if imp.config.CSVHasHeader && lineNum == 1 {
            continue
        }

        recipient, err := imp.parseCSVRecord(record, lineNum)
        if err != nil && !imp.config.SkipInvalid {
            return nil, err
        }

        if recipient != nil {
            recipients = append(recipients, recipient)
        }
    }

    return recipients, nil
}

func (imp *Importer) importTXTFromReader(ctx context.Context, reader io.Reader) ([]*models.Recipient, error) {
    scanner := bufio.NewScanner(reader)
    var recipients []*models.Recipient
    lineNum := 0

    for scanner.Scan() {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        default:
        }

        lineNum++
        line := scanner.Text()

        if imp.config.TrimSpaces {
            line = strings.TrimSpace(line)
        }

        if line == "" {
            continue
        }

        recipient, err := imp.parseTXTLine(line, lineNum)
        if err != nil && !imp.config.SkipInvalid {
            return nil, err
        }

        if recipient != nil {
            recipients = append(recipients, recipient)
        }
    }

    return recipients, nil
}

func (imp *Importer) parseCSVRecord(record []string, lineNum int) (*models.Recipient, error) {
    if len(record) == 0 {
        return nil, errors.New("empty record")
    }

    if imp.config.EmailColumn >= len(record) {
        return nil, errors.New("email column out of range")
    }

    email := record[imp.config.EmailColumn]
    if imp.config.TrimSpaces {
        email = strings.TrimSpace(email)
    }
    if imp.config.LowercaseEmails {
        email = strings.ToLower(email)
    }

    if email == "" {
        return nil, errors.New("empty email")
    }

    recipient := &models.Recipient{
        Email:     email,
        Status:    models.RecipientStatusPending,
        CreatedAt: time.Now(),
    }

    // Note: Name and CustomData fields don't exist in models.Recipient
    // If these fields are needed, they should be added to the model first

    if imp.config.EnableValidation {
        if err := imp.validator.ValidateEmail(email); err != nil {
            return nil, fmt.Errorf("validation failed: %w", err)
        }
    }

    return recipient, nil
}

func (imp *Importer) parseTXTLine(line string, lineNum int) (*models.Recipient, error) {
    email := imp.extractEmail(line)
    if email == "" {
        return nil, errors.New("no email found")
    }

    if imp.config.LowercaseEmails {
        email = strings.ToLower(email)
    }

    recipient := &models.Recipient{
        Email:     email,
        Status:    models.RecipientStatusPending,
        CreatedAt: time.Now(),
    }

    if imp.config.EnableValidation {
        if err := imp.validator.ValidateEmail(email); err != nil {
            return nil, fmt.Errorf("validation failed: %w", err)
        }
    }

    return recipient, nil
}

func (imp *Importer) extractEmail(line string) string {
    parts := strings.Fields(line)
    for _, part := range parts {
        if strings.Contains(part, "@") {
            return strings.TrimSpace(part)
        }
    }

    if strings.Contains(line, "@") {
        return strings.TrimSpace(line)
    }

    return ""
}

func (imp *Importer) detectFormat(filePath string) FileFormat {
    ext := strings.ToLower(filepath.Ext(filePath))

    switch ext {
    case ".csv":
        return FormatCSV
    case ".txt":
        return FormatTXT
    default:
        return imp.detectFormatByContent(filePath)
    }
}

func (imp *Importer) detectFormatByContent(filePath string) FileFormat {
    file, err := os.Open(filePath)
    if err != nil {
        return FormatUnknown
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    if !scanner.Scan() {
        return FormatUnknown
    }

    firstLine := scanner.Text()

    if strings.Contains(firstLine, ",") || strings.Contains(firstLine, ";") {
        return FormatCSV
    }

    if strings.Contains(firstLine, "@") {
        return FormatTXT
    }

    return FormatUnknown
}

func (imp *Importer) validateFile(filePath string) error {
    info, err := os.Stat(filePath)
    if err != nil {
        if os.IsNotExist(err) {
            return ErrFileNotFound
        }
        return err
    }

    if info.IsDir() {
        return errors.New("path is a directory, not a file")
    }

    if info.Size() == 0 {
        return ErrEmptyFile
    }

    if imp.config.MaxFileSize > 0 && info.Size() > imp.config.MaxFileSize {
        return fmt.Errorf("file size exceeds maximum: %d > %d", info.Size(), imp.config.MaxFileSize)
    }

    return nil
}

func (imp *Importer) ImportWithResult(ctx context.Context, filePath string) (*ImportResult, error) {
    result := &ImportResult{
        StartedAt: time.Now(),
        Errors:    make([]ImportError, 0),
    }

    recipients, err := imp.ImportFromFile(ctx, filePath)
    if err != nil {
        return result, err
    }

    result.Recipients = recipients
    result.ValidEmails = len(recipients)
    result.CompletedAt = time.Now()
    result.Duration = result.CompletedAt.Sub(result.StartedAt)

    return result, nil
}

func (imp *Importer) BatchImport(ctx context.Context, filePath string, batchCallback func([]*models.Recipient) error) error {
    format := imp.detectFormat(filePath)
    if format == FormatUnknown {
        return ErrInvalidFileFormat
    }

    file, err := os.Open(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    batch := make([]*models.Recipient, 0, imp.config.BatchSize)

    processBatch := func() error {
        if len(batch) == 0 {
            return nil
        }

        if err := batchCallback(batch); err != nil {
            return err
        }

        batch = make([]*models.Recipient, 0, imp.config.BatchSize)
        return nil
    }

    if format == FormatCSV {
        reader := csv.NewReader(file)
        reader.Comma = imp.config.CSVDelimiter
        lineNum := 0

        for {
            record, err := reader.Read()
            if err == io.EOF {
                break
            }
            if err != nil {
                continue
            }

            lineNum++
            if imp.config.CSVHasHeader && lineNum == 1 {
                continue
            }

            recipient, err := imp.parseCSVRecord(record, lineNum)
            if err != nil {
                continue
            }

            batch = append(batch, recipient)

            if len(batch) >= imp.config.BatchSize {
                if err := processBatch(); err != nil {
                    return err
                }
            }
        }
    } else if format == FormatTXT {
        scanner := bufio.NewScanner(file)
        lineNum := 0

        for scanner.Scan() {
            lineNum++
            line := scanner.Text()

            recipient, err := imp.parseTXTLine(line, lineNum)
            if err != nil {
                continue
            }

            batch = append(batch, recipient)

            if len(batch) >= imp.config.BatchSize {
                if err := processBatch(); err != nil {
                    return err
                }
            }
        }
    }

    return processBatch()
}

func (imp *Importer) SetConfig(config *ImportConfig) {
    imp.config = config
}

func (imp *Importer) GetConfig() *ImportConfig {
    return imp.config
}

func (imp *Importer) CountEmails(filePath string) (int, error) {
    format := imp.detectFormat(filePath)
    if format == FormatUnknown {
        return 0, ErrInvalidFileFormat
    }

    file, err := os.Open(filePath)
    if err != nil {
        return 0, err
    }
    defer file.Close()

    count := 0

    if format == FormatCSV {
        reader := csv.NewReader(file)
        reader.Comma = imp.config.CSVDelimiter
        lineNum := 0

        for {
            _, err := reader.Read()
            if err == io.EOF {
                break
            }
            if err != nil {
                continue
            }

            lineNum++
            if imp.config.CSVHasHeader && lineNum == 1 {
                continue
            }

            count++
        }
    } else if format == FormatTXT {
        scanner := bufio.NewScanner(file)

        for scanner.Scan() {
            line := strings.TrimSpace(scanner.Text())
            if line != "" && !strings.HasPrefix(line, "#") {
                count++
            }
        }
    }

    return count, nil
}

func (imp *Importer) PreviewFile(filePath string, maxLines int) ([]string, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    lines := make([]string, 0, maxLines)

    for scanner.Scan() && len(lines) < maxLines {
        lines = append(lines, scanner.Text())
    }

    return lines, nil
}
