package campaign

import (
    "bytes"
    "compress/gzip"
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "sync"
    "time"

    "email-campaign-system/internal/storage/cache"
    "email-campaign-system/internal/storage/files"
    "email-campaign-system/internal/storage/repository"
    "email-campaign-system/pkg/logger"
)

var (
    ErrStateCorrupted      = errors.New("state data corrupted")
    ErrCheckpointNotFound  = errors.New("checkpoint not found")
    ErrStateVersionMismatch = errors.New("state version mismatch")
    ErrInvalidStateData    = errors.New("invalid state data")
)

// ✅ Simple encryptor interface
type Encryptor interface {
    Encrypt(data []byte) ([]byte, error)
    Decrypt(data []byte) ([]byte, error)
}

type Persistence struct {
    repo          repository.CampaignRepository
    cache         cache.Cache
    fileStorage   files.Storage
    encryptor     Encryptor  // ✅ Fixed: Use interface instead of crypto.AESEncryptor
    log           logger.Logger  // ✅ Fixed: Interface, not pointer
    config        PersistenceConfig
    mu            sync.RWMutex
    checkpoints   map[string][]*Checkpoint
    compressor    *StateCompressor
}

type PersistenceConfig struct {
    StoragePath          string
    CheckpointInterval   time.Duration
    MaxCheckpoints       int
    MaxStateAge          time.Duration
    EnableCompression    bool
    EnableEncryption     bool
    EnableCache          bool
    BackupRetention      int
    AutoCleanup          bool
    CleanupInterval      time.Duration
}

type PersistedState struct {
    Version          int
    CampaignID       string
    State            *CampaignState
    ExecutionState   *ExecutionState
    Stats            *ExecutionStats
    Metadata         map[string]interface{}
    Timestamp        time.Time
    Checksum         string
    Compressed       bool
    Encrypted        bool
    Size             int64
}

type Checkpoint struct {
    ID             string
    CampaignID     string
    Version        int
    State          *PersistedState
    CreatedAt      time.Time
    Description    string
    Tags           []string
    RecipientIndex int64
    SuccessCount   int64
    FailedCount    int64
}

type StateSnapshot struct {
    ID          string
    CampaignID  string
    StateData   []byte
    Timestamp   time.Time
    Size        int64
    Compressed  bool
    Encrypted   bool
}

type StateHistory struct {
    CampaignID string
    States     []*PersistedState
    Total      int
    FromTime   time.Time
    ToTime     time.Time
}

type StateCompressor struct {
    enabled bool
    level   int
    mu      sync.RWMutex
}

const (
    currentStateVersion = 1
    stateFileExtension  = ".state"
    checksumAlgorithm   = "sha256"
)

func NewPersistence(
    repo repository.CampaignRepository,
    cache cache.Cache,
    fileStorage files.Storage,
    encryptor Encryptor,  // ✅ Fixed: Use interface
    log logger.Logger,    // ✅ Fixed: Interface
    config PersistenceConfig,
) *Persistence {
    if config.CheckpointInterval <= 0 {
        config.CheckpointInterval = 5 * time.Minute
    }
    if config.MaxCheckpoints <= 0 {
        config.MaxCheckpoints = 10
    }
    if config.MaxStateAge <= 0 {
        config.MaxStateAge = 30 * 24 * time.Hour
    }
    if config.BackupRetention <= 0 {
        config.BackupRetention = 7
    }
    if config.CleanupInterval <= 0 {
        config.CleanupInterval = 1 * time.Hour
    }

    p := &Persistence{
        repo:        repo,
        cache:       cache,
        fileStorage: fileStorage,
        encryptor:   encryptor,
        log:         log,
        config:      config,
        checkpoints: make(map[string][]*Checkpoint),
        compressor:  NewStateCompressor(config.EnableCompression),
    }

    if config.AutoCleanup {
        go p.autoCleanupLoop()
    }

    return p
}

func NewStateCompressor(enabled bool) *StateCompressor {
    return &StateCompressor{
        enabled: enabled,
        level:   gzip.BestCompression,
    }
}

func (p *Persistence) SaveState(ctx context.Context, campaignID string, state *CampaignState) error {
    p.mu.Lock()
    defer p.mu.Unlock()

    persistedState := &PersistedState{
        Version:    currentStateVersion,
        CampaignID: campaignID,
        State:      state.Clone(),
        Metadata:   make(map[string]interface{}),
        Timestamp:  time.Now(),
        Compressed: p.config.EnableCompression,
        Encrypted:  p.config.EnableEncryption,
    }

    data, err := p.serializeState(persistedState)
    if err != nil {
        return fmt.Errorf("failed to serialize state: %w", err)
    }

    if p.config.EnableCompression {
        data, err = p.compressor.Compress(data)
        if err != nil {
            return fmt.Errorf("failed to compress state: %w", err)
        }
    }

    if p.config.EnableEncryption && p.encryptor != nil {
        data, err = p.encryptor.Encrypt(data)
        if err != nil {
            return fmt.Errorf("failed to encrypt state: %w", err)
        }
    }

    persistedState.Size = int64(len(data))
    persistedState.Checksum = calculateHash(data)  // ✅ Fixed: Local function

    if err := p.saveToDatabase(ctx, campaignID, data, persistedState); err != nil {
        return fmt.Errorf("failed to save to database: %w", err)
    }

    if p.config.EnableCache {
        cacheKey := p.getCacheKey(campaignID)
        if err := p.cache.Set(ctx, cacheKey, data, 1*time.Hour); err != nil {
            p.log.Warn("failed to cache state", 
                logger.String("campaign_id", campaignID), 
                logger.String("error", err.Error()))
        }
    }

    if err := p.saveToFile(ctx, campaignID, data); err != nil {
        p.log.Warn("failed to save state to file", 
            logger.String("campaign_id", campaignID), 
            logger.String("error", err.Error()))
    }

    p.log.Debug("state saved", 
        logger.String("campaign_id", campaignID), 
        logger.Int64("size", persistedState.Size))

    return nil
}

func (p *Persistence) LoadState(ctx context.Context, campaignID string) (*CampaignState, error) {
    p.mu.RLock()
    defer p.mu.RUnlock()

    var data []byte
    var err error

    if p.config.EnableCache {
        cacheKey := p.getCacheKey(campaignID)
        cachedData, err := p.cache.Get(ctx, cacheKey)
        if err == nil && cachedData != nil {
            // ✅ Fixed: cachedData is already []byte, no type assertion needed
            data = cachedData
            p.log.Debug("state loaded from cache", 
                logger.String("campaign_id", campaignID))
        }
    }

    if data == nil {
        data, err = p.loadFromDatabase(ctx, campaignID)
        if err != nil {
            data, err = p.loadFromFile(ctx, campaignID)
            if err != nil {
                return nil, fmt.Errorf("failed to load state: %w", err)
            }
        }
    }

    if p.config.EnableEncryption && p.encryptor != nil {
        data, err = p.encryptor.Decrypt(data)
        if err != nil {
            return nil, fmt.Errorf("failed to decrypt state: %w", err)
        }
    }

    if p.config.EnableCompression {
        data, err = p.compressor.Decompress(data)
        if err != nil {
            return nil, fmt.Errorf("failed to decompress state: %w", err)
        }
    }

    persistedState, err := p.deserializeState(data)
    if err != nil {
        return nil, fmt.Errorf("failed to deserialize state: %w", err)
    }

    if persistedState.Version != currentStateVersion {
        return nil, fmt.Errorf("%w: expected %d, got %d", ErrStateVersionMismatch, currentStateVersion, persistedState.Version)
    }

    if err := p.validateChecksum(data, persistedState.Checksum); err != nil {
        return nil, fmt.Errorf("state validation failed: %w", err)
    }

    p.log.Debug("state loaded", 
        logger.String("campaign_id", campaignID))

    return persistedState.State, nil
}

func (p *Persistence) DeleteState(ctx context.Context, campaignID string) error {
    p.mu.Lock()
    defer p.mu.Unlock()

    if err := p.deleteFromDatabase(ctx, campaignID); err != nil {
        p.log.Error("failed to delete from database", 
            logger.String("campaign_id", campaignID), 
            logger.String("error", err.Error()))
    }

    if p.config.EnableCache {
        cacheKey := p.getCacheKey(campaignID)
        if err := p.cache.Delete(ctx, cacheKey); err != nil {
            p.log.Warn("failed to delete from cache", 
                logger.String("campaign_id", campaignID), 
                logger.String("error", err.Error()))
        }
    }

    if err := p.deleteFromFile(ctx, campaignID); err != nil {
        p.log.Warn("failed to delete file", 
            logger.String("campaign_id", campaignID), 
            logger.String("error", err.Error()))
    }

    delete(p.checkpoints, campaignID)

    p.log.Debug("state deleted", 
        logger.String("campaign_id", campaignID))

    return nil
}

func (p *Persistence) CreateCheckpoint(ctx context.Context, campaignID string, state *CampaignState, description string) (*Checkpoint, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    checkpoint := &Checkpoint{
        ID:          fmt.Sprintf("%s-%d", campaignID, time.Now().Unix()),
        CampaignID:  campaignID,
        Version:     currentStateVersion,
        CreatedAt:   time.Now(),
        Description: description,
        Tags:        []string{},
    }

    persistedState := &PersistedState{
        Version:    currentStateVersion,
        CampaignID: campaignID,
        State:      state.Clone(),
        Timestamp:  time.Now(),
    }

    checkpoint.State = persistedState

    if p.checkpoints[campaignID] == nil {
        p.checkpoints[campaignID] = make([]*Checkpoint, 0)
    }

    p.checkpoints[campaignID] = append(p.checkpoints[campaignID], checkpoint)

    if len(p.checkpoints[campaignID]) > p.config.MaxCheckpoints {
        p.checkpoints[campaignID] = p.checkpoints[campaignID][1:]
    }

    data, err := json.Marshal(checkpoint)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal checkpoint: %w", err)
    }

    checkpointPath := p.getCheckpointPath(campaignID, checkpoint.ID)
    if err := p.fileStorage.Write(ctx, checkpointPath, data); err != nil {
        p.log.Warn("failed to save checkpoint to file", 
            logger.String("checkpoint_id", checkpoint.ID), 
            logger.String("error", err.Error()))
    }

    p.log.Debug("checkpoint created", 
        logger.String("campaign_id", campaignID), 
        logger.String("checkpoint_id", checkpoint.ID))

    return checkpoint, nil
}

func (p *Persistence) RestoreCheckpoint(ctx context.Context, campaignID, checkpointID string) (*CampaignState, error) {
    p.mu.RLock()
    defer p.mu.RUnlock()

    checkpoints, exists := p.checkpoints[campaignID]
    if !exists || len(checkpoints) == 0 {
        checkpoints, err := p.loadCheckpointsFromFile(ctx, campaignID)
        if err != nil {
            return nil, fmt.Errorf("no checkpoints found: %w", err)
        }
        p.checkpoints[campaignID] = checkpoints
    }

    var targetCheckpoint *Checkpoint
    for _, cp := range p.checkpoints[campaignID] {
        if cp.ID == checkpointID {
            targetCheckpoint = cp
            break
        }
    }

    if targetCheckpoint == nil {
        return nil, ErrCheckpointNotFound
    }

    p.log.Debug("checkpoint restored", 
        logger.String("campaign_id", campaignID), 
        logger.String("checkpoint_id", checkpointID))

    return targetCheckpoint.State.State.Clone(), nil
}

func (p *Persistence) ListCheckpoints(ctx context.Context, campaignID string) ([]*Checkpoint, error) {
    p.mu.RLock()
    defer p.mu.RUnlock()

    checkpoints, exists := p.checkpoints[campaignID]
    if !exists || len(checkpoints) == 0 {
        checkpoints, err := p.loadCheckpointsFromFile(ctx, campaignID)
        if err != nil {
            return []*Checkpoint{}, nil
        }
        return checkpoints, nil
    }

    result := make([]*Checkpoint, len(checkpoints))
    copy(result, checkpoints)

    return result, nil
}

func (p *Persistence) GetStateHistory(ctx context.Context, campaignID string, from, to time.Time) (*StateHistory, error) {
    history := &StateHistory{
        CampaignID: campaignID,
        States:     make([]*PersistedState, 0),
        FromTime:   from,
        ToTime:     to,
    }

    return history, nil
}

func (p *Persistence) ExportState(ctx context.Context, campaignID string) ([]byte, error) {
    state, err := p.LoadState(ctx, campaignID)
    if err != nil {
        return nil, fmt.Errorf("failed to load state: %w", err)
    }

    export := &PersistedState{
        Version:    currentStateVersion,
        CampaignID: campaignID,
        State:      state,
        Timestamp:  time.Now(),
    }

    data, err := json.MarshalIndent(export, "", "  ")
    if err != nil {
        return nil, fmt.Errorf("failed to marshal export: %w", err)
    }

    return data, nil
}

func (p *Persistence) ImportState(ctx context.Context, data []byte) error {
    var persistedState PersistedState
    if err := json.Unmarshal(data, &persistedState); err != nil {
        return fmt.Errorf("failed to unmarshal import: %w", err)
    }

    if persistedState.Version != currentStateVersion {
        return ErrStateVersionMismatch
    }

    return p.SaveState(ctx, persistedState.CampaignID, persistedState.State)
}

func (p *Persistence) CleanupOldStates(ctx context.Context) error {
    p.mu.Lock()
    defer p.mu.Unlock()

    cutoffTime := time.Now().Add(-p.config.MaxStateAge)

    p.log.Debug("cleaning up old states", 
        logger.String("cutoff_time", cutoffTime.Format(time.RFC3339)))

    return nil
}

func (p *Persistence) serializeState(state *PersistedState) ([]byte, error) {
    return json.Marshal(state)
}

func (p *Persistence) deserializeState(data []byte) (*PersistedState, error) {
    var state PersistedState
    if err := json.Unmarshal(data, &state); err != nil {
        return nil, err
    }
    return &state, nil
}

func (p *Persistence) validateChecksum(data []byte, expectedChecksum string) error {
    actualChecksum := calculateHash(data)  // ✅ Fixed: Local function
    if actualChecksum != expectedChecksum {
        return ErrStateCorrupted
    }
    return nil
}

func (p *Persistence) saveToDatabase(ctx context.Context, campaignID string, data []byte, state *PersistedState) error {
    return nil
}

func (p *Persistence) loadFromDatabase(ctx context.Context, campaignID string) ([]byte, error) {
    return nil, errors.New("not implemented")
}

func (p *Persistence) deleteFromDatabase(ctx context.Context, campaignID string) error {
    return nil
}

func (p *Persistence) saveToFile(ctx context.Context, campaignID string, data []byte) error {
    filePath := p.getStatePath(campaignID)
    return p.fileStorage.Write(ctx, filePath, data)
}

func (p *Persistence) loadFromFile(ctx context.Context, campaignID string) ([]byte, error) {
    filePath := p.getStatePath(campaignID)
    return p.fileStorage.Read(ctx, filePath)
}

func (p *Persistence) deleteFromFile(ctx context.Context, campaignID string) error {
    filePath := p.getStatePath(campaignID)
    return p.fileStorage.Delete(ctx, filePath)
}

func (p *Persistence) loadCheckpointsFromFile(ctx context.Context, campaignID string) ([]*Checkpoint, error) {
    checkpointDir := filepath.Join(p.config.StoragePath, "checkpoints", campaignID)
    
    if _, err := os.Stat(checkpointDir); os.IsNotExist(err) {
        return []*Checkpoint{}, nil
    }

    files, err := os.ReadDir(checkpointDir)
    if err != nil {
        return nil, fmt.Errorf("failed to read checkpoint directory: %w", err)
    }

    checkpoints := make([]*Checkpoint, 0, len(files))
    for _, file := range files {
        if file.IsDir() {
            continue
        }

        filePath := filepath.Join(checkpointDir, file.Name())
        data, err := os.ReadFile(filePath)
        if err != nil {
            p.log.Warn("failed to read checkpoint file", 
                logger.String("file", file.Name()), 
                logger.String("error", err.Error()))
            continue
        }

        var checkpoint Checkpoint
        if err := json.Unmarshal(data, &checkpoint); err != nil {
            p.log.Warn("failed to unmarshal checkpoint", 
                logger.String("file", file.Name()), 
                logger.String("error", err.Error()))
            continue
        }

        checkpoints = append(checkpoints, &checkpoint)
    }

    return checkpoints, nil
}

func (p *Persistence) getStatePath(campaignID string) string {
    return filepath.Join(p.config.StoragePath, "states", campaignID+stateFileExtension)
}

func (p *Persistence) getCheckpointPath(campaignID, checkpointID string) string {
    return filepath.Join(p.config.StoragePath, "checkpoints", campaignID, checkpointID+".json")
}

func (p *Persistence) getCacheKey(campaignID string) string {
    return fmt.Sprintf("campaign:state:%s", campaignID)
}

func (p *Persistence) autoCleanupLoop() {
    ticker := time.NewTicker(p.config.CleanupInterval)
    defer ticker.Stop()

    for range ticker.C {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
        if err := p.CleanupOldStates(ctx); err != nil {
            p.log.Error("auto cleanup failed", 
                logger.String("error", err.Error()))
        }
        cancel()
    }
}

func (sc *StateCompressor) Compress(data []byte) ([]byte, error) {
    if !sc.enabled {
        return data, nil
    }

    sc.mu.Lock()
    defer sc.mu.Unlock()

    var buf bytes.Buffer
    writer, err := gzip.NewWriterLevel(&buf, sc.level)
    if err != nil {
        return nil, err
    }

    if _, err := writer.Write(data); err != nil {
        writer.Close()
        return nil, err
    }

    if err := writer.Close(); err != nil {
        return nil, err
    }

    return buf.Bytes(), nil
}

func (sc *StateCompressor) Decompress(data []byte) ([]byte, error) {
    if !sc.enabled {
        return data, nil
    }

    sc.mu.RLock()
    defer sc.mu.RUnlock()

    reader, err := gzip.NewReader(bytes.NewReader(data))
    if err != nil {
        return nil, err
    }
    defer reader.Close()

    var buf bytes.Buffer
    if _, err := io.Copy(&buf, reader); err != nil {
        return nil, err
    }

    return buf.Bytes(), nil
}

func (ps *PersistedState) IsExpired(maxAge time.Duration) bool {
    return time.Since(ps.Timestamp) > maxAge
}

func (ps *PersistedState) Clone() *PersistedState {
    metadata := make(map[string]interface{}, len(ps.Metadata))
    for k, v := range ps.Metadata {
        metadata[k] = v
    }

    return &PersistedState{
        Version:    ps.Version,
        CampaignID: ps.CampaignID,
        State:      ps.State.Clone(),
        Metadata:   metadata,
        Timestamp:  ps.Timestamp,
        Checksum:   ps.Checksum,
        Compressed: ps.Compressed,
        Encrypted:  ps.Encrypted,
        Size:       ps.Size,
    }
}

func calculateHash(data []byte) string {
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:])
}
