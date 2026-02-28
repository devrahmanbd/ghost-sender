package recipient

import (
    "crypto/md5"
    "encoding/hex"
    "strings"
    "sync"
    "time"

    "email-campaign-system/internal/models"
)

type Deduplicator struct {
    config *DeduplicationConfig
    mu     sync.RWMutex
}

type DeduplicationConfig struct {
    Strategy          DeduplicationStrategy
    CaseSensitive     bool
    TrimSpaces        bool
    NormalizeEmail    bool
    IgnoreSubaddress  bool
    KeepFirst         bool
    MergeMetadata     bool
    TrackDuplicates   bool
}

type DeduplicationStrategy string

const (
    StrategyEmailOnly       DeduplicationStrategy = "email_only"
    StrategyEmailAndName    DeduplicationStrategy = "email_and_name"
    StrategyEmailAndDomain  DeduplicationStrategy = "email_and_domain"
    StrategyNormalized      DeduplicationStrategy = "normalized"
    StrategyStrict          DeduplicationStrategy = "strict"
)

type DeduplicationResult struct {
    OriginalCount    int
    UniqueCount      int
    DuplicateCount   int
    RemovedCount     int
    UniqueRecipients []*models.Recipient
    Duplicates       map[string][]*models.Recipient
    ProcessedAt      time.Time
    Duration         time.Duration
}

type DuplicateGroup struct {
    Key        string
    Recipients []*models.Recipient
    Count      int
}

func NewDeduplicator() *Deduplicator {
    return &Deduplicator{
        config: DefaultDeduplicationConfig(),
    }
}

func DefaultDeduplicationConfig() *DeduplicationConfig {
    return &DeduplicationConfig{
        Strategy:         StrategyNormalized,
        CaseSensitive:    false,
        TrimSpaces:       true,
        NormalizeEmail:   true,
        IgnoreSubaddress: true,
        KeepFirst:        true,
        MergeMetadata:    false, // Disabled since CustomData field doesn't exist
        TrackDuplicates:  false,
    }
}

func (d *Deduplicator) RemoveDuplicates(recipients []*models.Recipient) []*models.Recipient {
    if len(recipients) == 0 {
        return recipients
    }

    d.mu.Lock()
    defer d.mu.Unlock()

    seen := make(map[string]bool)
    unique := make([]*models.Recipient, 0, len(recipients))

    for _, recipient := range recipients {
        if recipient == nil {
            continue
        }

        key := d.generateKey(recipient)
        
        if !seen[key] {
            seen[key] = true
            unique = append(unique, recipient)
        }
    }

    return unique
}

func (d *Deduplicator) RemoveDuplicatesWithResult(recipients []*models.Recipient) *DeduplicationResult {
    startTime := time.Now()

    result := &DeduplicationResult{
        OriginalCount: len(recipients),
        Duplicates:    make(map[string][]*models.Recipient),
        ProcessedAt:   startTime,
    }

    if len(recipients) == 0 {
        result.Duration = time.Since(startTime)
        return result
    }

    seen := make(map[string]*models.Recipient)
    duplicates := make(map[string][]*models.Recipient)

    for _, recipient := range recipients {
        if recipient == nil {
            continue
        }

        key := d.generateKey(recipient)

        if existing, exists := seen[key]; exists {
            duplicates[key] = append(duplicates[key], recipient)
            
            if d.config.MergeMetadata {
                d.mergeRecipientData(existing, recipient)
            }
        } else {
            seen[key] = recipient
        }
    }

    unique := make([]*models.Recipient, 0, len(seen))
    for _, recipient := range seen {
        unique = append(unique, recipient)
    }

    result.UniqueRecipients = unique
    result.UniqueCount = len(unique)
    result.DuplicateCount = len(recipients) - len(unique)
    result.RemovedCount = result.DuplicateCount

    if d.config.TrackDuplicates {
        result.Duplicates = duplicates
    }

    result.Duration = time.Since(startTime)

    return result
}

func (d *Deduplicator) DetectDuplicates(recipients []*models.Recipient) map[string][]*models.Recipient {
    if len(recipients) == 0 {
        return make(map[string][]*models.Recipient)
    }

    groups := make(map[string][]*models.Recipient)

    for _, recipient := range recipients {
        if recipient == nil {
            continue
        }

        key := d.generateKey(recipient)
        groups[key] = append(groups[key], recipient)
    }

    duplicates := make(map[string][]*models.Recipient)
    for key, group := range groups {
        if len(group) > 1 {
            duplicates[key] = group
        }
    }

    return duplicates
}

func (d *Deduplicator) FindDuplicateGroups(recipients []*models.Recipient) []*DuplicateGroup {
    duplicateMap := d.DetectDuplicates(recipients)

    groups := make([]*DuplicateGroup, 0, len(duplicateMap))
    for key, recs := range duplicateMap {
        groups = append(groups, &DuplicateGroup{
            Key:        key,
            Recipients: recs,
            Count:      len(recs),
        })
    }

    return groups
}

func (d *Deduplicator) generateKey(recipient *models.Recipient) string {
    switch d.config.Strategy {
    case StrategyEmailOnly:
        return d.normalizeForKey(recipient.Email)
    
    case StrategyEmailAndName:
        // Name field doesn't exist in models.Recipient, fall back to email only
        email := d.normalizeForKey(recipient.Email)
        return email
    
    case StrategyEmailAndDomain:
        email := d.normalizeForKey(recipient.Email)
        parts := strings.Split(email, "@")
        if len(parts) == 2 {
            return d.hashKey(parts[0] + "|" + parts[1])
        }
        return d.hashKey(email)
    
    case StrategyNormalized:
        return d.normalizeForKey(recipient.Email)
    
    case StrategyStrict:
        return recipient.Email
    
    default:
        return d.normalizeForKey(recipient.Email)
    }
}

func (d *Deduplicator) normalizeForKey(value string) string {
    if d.config.TrimSpaces {
        value = strings.TrimSpace(value)
    }

    if !d.config.CaseSensitive {
        value = strings.ToLower(value)
    }

    if d.config.NormalizeEmail && strings.Contains(value, "@") {
        value = d.normalizeEmailAddress(value)
    }

    return value
}

func (d *Deduplicator) normalizeEmailAddress(email string) string {
    email = strings.ToLower(strings.TrimSpace(email))

    if d.config.IgnoreSubaddress {
        parts := strings.Split(email, "@")
        if len(parts) == 2 {
            localPart := parts[0]
            if idx := strings.Index(localPart, "+"); idx != -1 {
                localPart = localPart[:idx]
            }
            email = localPart + "@" + parts[1]
        }
    }

    parts := strings.Split(email, "@")
    if len(parts) == 2 {
        domain := parts[1]
        if domain == "gmail.com" || domain == "googlemail.com" {
            localPart := strings.ReplaceAll(parts[0], ".", "")
            email = localPart + "@gmail.com"
        }
    }

    return email
}

func (d *Deduplicator) hashKey(value string) string {
    hash := md5.Sum([]byte(value))
    return hex.EncodeToString(hash[:])
}

func (d *Deduplicator) mergeRecipientData(target, source *models.Recipient) {
    // models.Recipient doesn't have Name or CustomData fields
    // This function is kept for interface compatibility but does nothing
    // If additional fields are added to models.Recipient in the future, 
    // merge logic can be implemented here
}

func (d *Deduplicator) DeduplicateByEmail(emails []string) []string {
    if len(emails) == 0 {
        return emails
    }

    seen := make(map[string]bool)
    unique := make([]string, 0, len(emails))

    for _, email := range emails {
        key := d.normalizeForKey(email)
        if !seen[key] {
            seen[key] = true
            unique = append(unique, email)
        }
    }

    return unique
}

func (d *Deduplicator) CountDuplicates(recipients []*models.Recipient) int {
    if len(recipients) == 0 {
        return 0
    }

    seen := make(map[string]bool)
    duplicates := 0

    for _, recipient := range recipients {
        if recipient == nil {
            continue
        }

        key := d.generateKey(recipient)
        if seen[key] {
            duplicates++
        } else {
            seen[key] = true
        }
    }

    return duplicates
}

func (d *Deduplicator) HasDuplicates(recipients []*models.Recipient) bool {
    if len(recipients) <= 1 {
        return false
    }

    seen := make(map[string]bool)

    for _, recipient := range recipients {
        if recipient == nil {
            continue
        }

        key := d.generateKey(recipient)
        if seen[key] {
            return true
        }
        seen[key] = true
    }

    return false
}

func (d *Deduplicator) GetDuplicateStats(recipients []*models.Recipient) map[string]interface{} {
    result := d.RemoveDuplicatesWithResult(recipients)

    return map[string]interface{}{
        "original_count":  result.OriginalCount,
        "unique_count":    result.UniqueCount,
        "duplicate_count": result.DuplicateCount,
        "removed_count":   result.RemovedCount,
        "duplicate_rate":  d.calculateDuplicateRate(result),
        "processed_at":    result.ProcessedAt,
        "duration_ms":     result.Duration.Milliseconds(),
    }
}

func (d *Deduplicator) calculateDuplicateRate(result *DeduplicationResult) float64 {
    if result.OriginalCount == 0 {
        return 0.0
    }
    return float64(result.DuplicateCount) / float64(result.OriginalCount) * 100
}

func (d *Deduplicator) SetConfig(config *DeduplicationConfig) {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.config = config
}

func (d *Deduplicator) GetConfig() *DeduplicationConfig {
    d.mu.RLock()
    defer d.mu.RUnlock()
    return d.config
}

func (d *Deduplicator) SetStrategy(strategy DeduplicationStrategy) {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.config.Strategy = strategy
}

func (d *Deduplicator) IsDuplicate(recipient1, recipient2 *models.Recipient) bool {
    if recipient1 == nil || recipient2 == nil {
        return false
    }

    key1 := d.generateKey(recipient1)
    key2 := d.generateKey(recipient2)

    return key1 == key2
}

func (d *Deduplicator) MergeDuplicateGroups(groups []*DuplicateGroup) []*models.Recipient {
    merged := make([]*models.Recipient, 0, len(groups))

    for _, group := range groups {
        if len(group.Recipients) == 0 {
            continue
        }

        base := group.Recipients[0]
        
        for i := 1; i < len(group.Recipients); i++ {
            d.mergeRecipientData(base, group.Recipients[i])
        }

        merged = append(merged, base)
    }

    return merged
}

func (d *Deduplicator) CompareRecipients(a, b *models.Recipient) bool {
    if a == nil || b == nil {
        return false
    }

    emailMatch := d.normalizeForKey(a.Email) == d.normalizeForKey(b.Email)

    if d.config.Strategy == StrategyEmailOnly || d.config.Strategy == StrategyNormalized {
        return emailMatch
    }

    if d.config.Strategy == StrategyEmailAndName {
        // Name field doesn't exist in models.Recipient, fall back to email only
        return emailMatch
    }

    return emailMatch
}

func (d *Deduplicator) FilterDuplicates(recipients []*models.Recipient, reference []*models.Recipient) []*models.Recipient {
    if len(recipients) == 0 {
        return recipients
    }

    referenceKeys := make(map[string]bool)
    for _, ref := range reference {
        if ref != nil {
            key := d.generateKey(ref)
            referenceKeys[key] = true
        }
    }

    unique := make([]*models.Recipient, 0, len(recipients))
    for _, recipient := range recipients {
        if recipient == nil {
            continue
        }

        key := d.generateKey(recipient)
        if !referenceKeys[key] {
            unique = append(unique, recipient)
        }
    }

    return unique
}

func (d *Deduplicator) Clear() {
    d.mu.Lock()
    defer d.mu.Unlock()
}
