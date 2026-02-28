// backend/internal/core/recipient/manager.go
package recipient

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"email-campaign-system/internal/models"
	"email-campaign-system/internal/storage/repository"
	"email-campaign-system/pkg/logger"
)

var (
	ErrRecipientNotFound  = errors.New("recipient not found")
	ErrInvalidRecipient   = errors.New("invalid recipient data")
	ErrDuplicateRecipient = errors.New("duplicate recipient")
	ErrRecipientPoolEmpty = errors.New("recipient pool is empty")
	ErrInvalidCampaignID  = errors.New("invalid campaign ID")
)

type RecipientManager struct {
	mu            sync.RWMutex
	repo          repository.RecipientRepository
	validator     *Validator
	deduplicator  *Deduplicator
	importer      *Importer
	logger        logger.Logger
	config        *ManagerConfig
	cache         map[string]*models.Recipient
	campaignCache map[string][]*models.Recipient
	stats         *RecipientStats
}

type ManagerConfig struct {
	EnableCache              bool
	CacheSize                int
	CacheTTL                 time.Duration
	BatchSize                int
	EnableDedup              bool
	EnableValidation         bool
	MaxRecipientsPerCampaign int
}

type RecipientStats struct {
	mu                sync.RWMutex
	TotalRecipients   int64
	ActiveRecipients  int64
	SentRecipients    int64
	FailedRecipients  int64
	PendingRecipients int64
	ByCampaign        map[string]*CampaignRecipientStats
	LastUpdated       time.Time
}

type CampaignRecipientStats struct {
	CampaignID  string
	Total       int64
	Sent        int64
	Failed      int64
	Pending     int64
	SuccessRate float64
	LastSentAt  time.Time
}

type RecipientFilter struct {
	CampaignID string
	Status     models.RecipientStatus
	Email      string
	SearchTerm string
	Limit      int
	Offset     int
	SortBy     string
	SortOrder  string
	DateFrom   time.Time
	DateTo     time.Time
}

type BulkOperation struct {
	Operation    string
	RecipientIDs []string
	CampaignID   string
	Status       models.RecipientStatus
}

func toRepoRecipient(m *models.Recipient) *repository.Recipient {
	if m == nil {
		return nil
	}
	return &repository.Recipient{
		ID:        m.ID,
		ListID:    m.CampaignID,
		Email:     m.Email,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Status:    string(m.Status),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func toModelRecipient(r *repository.Recipient) *models.Recipient {
	if r == nil {
		return nil
	}
	return &models.Recipient{
		ID:         r.ID,
		CampaignID: r.ListID,
		Email:      r.Email,
		Status:     models.RecipientStatus(r.Status),
		SentAt:     nil,
		FailedAt:   nil,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

func toModelRecipients(repos []*repository.Recipient) []*models.Recipient {
	result := make([]*models.Recipient, len(repos))
	for i, r := range repos {
		result[i] = toModelRecipient(r)
	}
	return result
}

func toRepoFilter(f *RecipientFilter) *repository.RecipientFilter {
	if f == nil {
		return &repository.RecipientFilter{}
	}
	return &repository.RecipientFilter{
		ListIDs:   []string{f.CampaignID},
		Status:    []string{string(f.Status)},
		Emails:    []string{f.Email},
		Limit:     f.Limit,
		Offset:    f.Offset,
		SortBy:    f.SortBy,
		SortOrder: f.SortOrder,
	}
}

func NewRecipientManager(
	repo repository.RecipientRepository,
	validator *Validator,
	logger logger.Logger,
) *RecipientManager {
	config := DefaultManagerConfig()

	rm := &RecipientManager{
		repo:          repo,
		validator:     validator,
		deduplicator:  NewDeduplicator(),
		importer:      NewImporter(validator, logger),
		logger:        logger,
		config:        config,
		cache:         make(map[string]*models.Recipient),
		campaignCache: make(map[string][]*models.Recipient),
		stats: &RecipientStats{
			ByCampaign: make(map[string]*CampaignRecipientStats),
		},
	}

	return rm
}

func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		EnableCache:              true,
		CacheSize:                10000,
		CacheTTL:                 5 * time.Minute,
		BatchSize:                1000,
		EnableDedup:              true,
		EnableValidation:         true,
		MaxRecipientsPerCampaign: 1000000,
	}
}

func (rm *RecipientManager) AddRecipient(ctx context.Context, recipient *models.Recipient) error {
	fmt.Printf("🟢 DEBUG Manager.AddRecipient: email=%s campaign=%s\n", recipient.Email, recipient.CampaignID)
	if recipient == nil {
		return ErrInvalidRecipient
	}

	if rm.config.EnableValidation {
		fmt.Printf("🟢 DEBUG calling validator.ValidateRecipient\n")
		if err := rm.validator.ValidateRecipient(recipient); err != nil {
			fmt.Printf("🔴 DEBUG ValidateRecipient ERROR: %v\n", err)
			return fmt.Errorf("validation failed: %w", err)
		}
		fmt.Printf("🟢 DEBUG ValidateRecipient passed\n")
	}

	fmt.Printf("🟢 DEBUG EnableDedup=%v\n", rm.config.EnableDedup)
	if rm.config.EnableDedup {
		fmt.Printf("🟢 DEBUG running dedup check: campaignID=%s email=%s\n", recipient.CampaignID, recipient.Email)
		filter := &repository.RecipientFilter{
			ListIDs: []string{recipient.CampaignID},
			Emails:  []string{recipient.Email},
			Limit:   1,
		}
		fmt.Printf("🟢 DEBUG calling repo.List for dedup\n")
		existing, _, err := rm.repo.List(ctx, filter)
		if err != nil {
			fmt.Printf("🔴 DEBUG repo.List ERROR: %v\n", err)
			return err
		}
		fmt.Printf("🟢 DEBUG repo.List returned %d existing\n", len(existing))
		if len(existing) > 0 {
			fmt.Printf("🔴 DEBUG duplicate found, returning ErrDuplicateRecipient\n")
			return ErrDuplicateRecipient
		}
	}

	fmt.Printf("🟢 DEBUG calling repo.Create\n")
	if err := rm.repo.Create(ctx, toRepoRecipient(recipient)); err != nil {
		fmt.Printf("🔴 DEBUG repo.Create ERROR: %v\n", err)
		return err
	}
	fmt.Printf("🟢 DEBUG repo.Create SUCCESS\n")

	if rm.config.EnableCache {
		rm.addToCache(recipient)
	}

	rm.updateStats(recipient, "add")

	rm.logger.Debug(fmt.Sprintf("recipient added: campaign_id=%s, email=%s",
		recipient.CampaignID, recipient.Email))

	return nil
}


func (rm *RecipientManager) AddRecipients(ctx context.Context, recipients []*models.Recipient) (int, error) {
	if len(recipients) == 0 {
		return 0, nil
	}

	if rm.config.EnableValidation {
		validated := make([]*models.Recipient, 0, len(recipients))
		for _, r := range recipients {
			if err := rm.validator.ValidateRecipient(r); err != nil {
				rm.logger.Warn(fmt.Sprintf("invalid recipient skipped: email=%s, error=%v",
					r.Email, err))
				continue
			}
			validated = append(validated, r)
		}
		recipients = validated
	}

	if rm.config.EnableDedup {
		recipients = rm.deduplicator.RemoveDuplicates(recipients)
	}

	added := 0
	for i := 0; i < len(recipients); i += rm.config.BatchSize {
		end := i + rm.config.BatchSize
		if end > len(recipients) {
			end = len(recipients)
		}

		batch := recipients[i:end]

		for _, r := range batch {
			if err := rm.repo.Create(ctx, toRepoRecipient(r)); err != nil {
				rm.logger.Error(fmt.Sprintf("insert failed: %v", err))
				continue
			}
			added++
			if rm.config.EnableCache {
				rm.addToCache(r)
			}
		}
	}

	rm.logger.Info(fmt.Sprintf("recipients added: total=%d, added=%d",
		len(recipients), added))

	return added, nil
}

func (rm *RecipientManager) GetRecipient(ctx context.Context, id string) (*models.Recipient, error) {
	if id == "" {
		return nil, ErrInvalidRecipient
	}

	if rm.config.EnableCache {
		if cached := rm.getFromCache(id); cached != nil {
			return cached, nil
		}
	}

	recipient, err := rm.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	model := toModelRecipient(recipient)

	if rm.config.EnableCache {
		rm.addToCache(model)
	}

	return model, nil
}

func (rm *RecipientManager) GetRecipients(ctx context.Context, filter *RecipientFilter) ([]*models.Recipient, error) {
	if filter == nil {
		filter = &RecipientFilter{
			Limit:  100,
			Offset: 0,
		}
	}

	recipients, _, err := rm.repo.List(ctx, toRepoFilter(filter))
	if err != nil {
		return nil, err
	}

	return toModelRecipients(recipients), nil
}

func (rm *RecipientManager) GetByEmail(ctx context.Context, campaignID, email string) (*models.Recipient, error) {
	if email == "" {
		return nil, ErrInvalidRecipient
	}

	recipient, err := rm.repo.GetByEmail(ctx, campaignID, email)
	if err != nil {
		return nil, err
	}

	return toModelRecipient(recipient), nil
}

func (rm *RecipientManager) GetByCampaign(ctx context.Context, campaignID string) ([]*models.Recipient, error) {
	if campaignID == "" {
		return nil, ErrInvalidCampaignID
	}

	if rm.config.EnableCache {
		rm.mu.RLock()
		cached, exists := rm.campaignCache[campaignID]
		rm.mu.RUnlock()
		if exists && cached != nil {
			return cached, nil
		}
	}

	filter := &repository.RecipientFilter{
		ListIDs: []string{campaignID},
	}
	recipients, _, err := rm.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	models := toModelRecipients(recipients)

	if rm.config.EnableCache {
		rm.mu.Lock()
		rm.campaignCache[campaignID] = models
		rm.mu.Unlock()
	}

	return models, nil
}

func (rm *RecipientManager) UpdateRecipient(ctx context.Context, recipient *models.Recipient) error {
	if recipient == nil || recipient.ID == "" {
		return ErrInvalidRecipient
	}

	if err := rm.repo.Update(ctx, toRepoRecipient(recipient)); err != nil {
		return err
	}

	if rm.config.EnableCache {
		rm.removeFromCache(recipient.ID)
	}

	rm.logger.Debug(fmt.Sprintf("recipient updated: id=%s", recipient.ID))

	return nil
}

func (rm *RecipientManager) DeleteRecipient(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidRecipient
	}

	if err := rm.repo.Delete(ctx, id); err != nil {
		return err
	}

	if rm.config.EnableCache {
		rm.removeFromCache(id)
	}

	rm.logger.Debug(fmt.Sprintf("recipient deleted: id=%s", id))

	return nil
}

func (rm *RecipientManager) DeleteRecipients(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	deleted := 0
	for i := 0; i < len(ids); i += rm.config.BatchSize {
		end := i + rm.config.BatchSize
		if end > len(ids) {
			end = len(ids)
		}

		batch := ids[i:end]

		for _, id := range batch {
			if err := rm.repo.Delete(ctx, id); err != nil {
				rm.logger.Error(fmt.Sprintf("delete failed: %v", err))
				continue
			}
			deleted++
			if rm.config.EnableCache {
				rm.removeFromCache(id)
			}
		}
	}

	rm.logger.Info(fmt.Sprintf("recipients deleted: count=%d", deleted))

	return deleted, nil
}

func (rm *RecipientManager) DeleteByCampaign(ctx context.Context, campaignID string) (int, error) {
	if campaignID == "" {
		return 0, ErrInvalidCampaignID
	}

	recipients, err := rm.GetByCampaign(ctx, campaignID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, r := range recipients {
		if err := rm.repo.Delete(ctx, r.ID); err != nil {
			continue
		}
		count++
	}

	if rm.config.EnableCache {
		rm.mu.Lock()
		delete(rm.campaignCache, campaignID)
		rm.mu.Unlock()
	}

	rm.logger.Info(fmt.Sprintf("campaign recipients deleted: campaign_id=%s, count=%d",
		campaignID, count))

	return count, nil
}

func (rm *RecipientManager) CountRecipients(ctx context.Context, filter *RecipientFilter) (int64, error) {
	count, err := rm.repo.Count(ctx, toRepoFilter(filter))
	if err != nil {
		return 0, err
	}

	return int64(count), nil
}

func (rm *RecipientManager) GetPending(ctx context.Context, campaignID string, limit int) ([]*models.Recipient, error) {
	if campaignID == "" {
		return nil, ErrInvalidCampaignID
	}

	filter := &RecipientFilter{
		CampaignID: campaignID,
		Status:     models.RecipientStatusPending,
		Limit:      limit,
	}

	recipients, _, err := rm.repo.List(ctx, toRepoFilter(filter))
	if err != nil {
		return nil, err
	}

	return toModelRecipients(recipients), nil
}

func (rm *RecipientManager) GetSent(ctx context.Context, campaignID string) ([]*models.Recipient, error) {
	if campaignID == "" {
		return nil, ErrInvalidCampaignID
	}

	filter := &RecipientFilter{
		CampaignID: campaignID,
		Status:     models.RecipientStatusSent,
	}

	recipients, _, err := rm.repo.List(ctx, toRepoFilter(filter))
	if err != nil {
		return nil, err
	}

	return toModelRecipients(recipients), nil
}

func (rm *RecipientManager) GetFailed(ctx context.Context, campaignID string) ([]*models.Recipient, error) {
	if campaignID == "" {
		return nil, ErrInvalidCampaignID
	}

	filter := &RecipientFilter{
		CampaignID: campaignID,
		Status:     models.RecipientStatusFailed,
	}

	recipients, _, err := rm.repo.List(ctx, toRepoFilter(filter))
	if err != nil {
		return nil, err
	}

	return toModelRecipients(recipients), nil
}

func (rm *RecipientManager) MarkAsSent(ctx context.Context, id string) error {
	recipient, err := rm.GetRecipient(ctx, id)
	if err != nil {
		return err
	}

	recipient.Status = models.RecipientStatusSent
	now := time.Now()
	recipient.SentAt = &now

	return rm.UpdateRecipient(ctx, recipient)
}

func (rm *RecipientManager) MarkAsFailed(ctx context.Context, id string, reason string) error {
	recipient, err := rm.GetRecipient(ctx, id)
	if err != nil {
		return err
	}

	recipient.Status = models.RecipientStatusFailed
	now := time.Now()
	recipient.FailedAt = &now

	return rm.UpdateRecipient(ctx, recipient)
}

func (rm *RecipientManager) ResetStatus(ctx context.Context, campaignID string) (int, error) {
	if campaignID == "" {
		return 0, ErrInvalidCampaignID
	}

	recipients, err := rm.GetByCampaign(ctx, campaignID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, r := range recipients {
		r.Status = models.RecipientStatusPending
		r.SentAt = nil
		r.FailedAt = nil
		if err := rm.UpdateRecipient(ctx, r); err != nil {
			continue
		}
		count++
	}

	if rm.config.EnableCache {
		rm.mu.Lock()
		delete(rm.campaignCache, campaignID)
		rm.mu.Unlock()
	}

	rm.logger.Info(fmt.Sprintf("recipient status reset: campaign_id=%s, count=%d",
		campaignID, count))

	return count, nil
}

func (rm *RecipientManager) GetStatistics(ctx context.Context, campaignID string) (*CampaignRecipientStats, error) {
	if campaignID == "" {
		return nil, ErrInvalidCampaignID
	}

	rm.stats.mu.RLock()
	stats, exists := rm.stats.ByCampaign[campaignID]
	rm.stats.mu.RUnlock()

	if exists && time.Since(rm.stats.LastUpdated) < rm.config.CacheTTL {
		return stats, nil
	}

	total, err := rm.repo.Count(ctx, toRepoFilter(&RecipientFilter{CampaignID: campaignID}))
	if err != nil {
		return nil, err
	}

	sent, err := rm.repo.Count(ctx, toRepoFilter(&RecipientFilter{
		CampaignID: campaignID,
		Status:     models.RecipientStatusSent,
	}))
	if err != nil {
		return nil, err
	}

	failed, err := rm.repo.Count(ctx, toRepoFilter(&RecipientFilter{
		CampaignID: campaignID,
		Status:     models.RecipientStatusFailed,
	}))
	if err != nil {
		return nil, err
	}

	pending := total - sent - failed

	var successRate float64
	if total > 0 {
		successRate = float64(sent) / float64(total) * 100
	}

	stats = &CampaignRecipientStats{
		CampaignID:  campaignID,
		Total:       int64(total),
		Sent:        int64(sent),
		Failed:      int64(failed),
		Pending:     int64(pending),
		SuccessRate: successRate,
		LastSentAt:  time.Now(),
	}

	rm.stats.mu.Lock()
	rm.stats.ByCampaign[campaignID] = stats
	rm.stats.LastUpdated = time.Now()
	rm.stats.mu.Unlock()

	return stats, nil
}

func (rm *RecipientManager) ImportFromFile(ctx context.Context, campaignID, filePath string) (int, error) {
	if campaignID == "" {
		return 0, ErrInvalidCampaignID
	}

	recipients, err := rm.importer.ImportFromFile(ctx, filePath)
	if err != nil {
		return 0, err
	}

	for _, r := range recipients {
		r.CampaignID = campaignID
	}

	added, err := rm.AddRecipients(ctx, recipients)
	if err != nil {
		return 0, err
	}

	rm.logger.Info(fmt.Sprintf("recipients imported: campaign_id=%s, file=%s, count=%d",
		campaignID, filePath, added))

	return added, nil
}

func (rm *RecipientManager) BulkOperation(ctx context.Context, op *BulkOperation) (int, error) {
	if op == nil {
		return 0, errors.New("operation is nil")
	}

	switch op.Operation {
	case "delete":
		return rm.DeleteRecipients(ctx, op.RecipientIDs)
	case "reset":
		return rm.ResetStatus(ctx, op.CampaignID)
	case "mark_sent":
		return rm.bulkMarkStatus(ctx, op.RecipientIDs, models.RecipientStatusSent)
	case "mark_failed":
		return rm.bulkMarkStatus(ctx, op.RecipientIDs, models.RecipientStatusFailed)
	default:
		return 0, fmt.Errorf("unknown operation: %s", op.Operation)
	}
}

func (rm *RecipientManager) bulkMarkStatus(ctx context.Context, ids []string, status models.RecipientStatus) (int, error) {
	updated := 0
	for _, id := range ids {
		recipient, err := rm.GetRecipient(ctx, id)
		if err != nil {
			continue
		}

		recipient.Status = status
		now := time.Now()
		if status == models.RecipientStatusSent {
			recipient.SentAt = &now
		} else if status == models.RecipientStatusFailed {
			recipient.FailedAt = &now
		}

		if err := rm.UpdateRecipient(ctx, recipient); err != nil {
			continue
		}

		updated++
	}

	return updated, nil
}

func (rm *RecipientManager) ClearCache() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.cache = make(map[string]*models.Recipient)
	rm.campaignCache = make(map[string][]*models.Recipient)

	rm.logger.Debug("recipient cache cleared")
}

func (rm *RecipientManager) BulkDelete(ctx context.Context, ids []string) (int, error) {
	return rm.DeleteRecipients(ctx, ids)
}

func (rm *RecipientManager) GetCacheStats() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return map[string]interface{}{
		"cache_size":          len(rm.cache),
		"campaign_cache_size": len(rm.campaignCache),
		"cache_enabled":       rm.config.EnableCache,
	}
}

func (rm *RecipientManager) addToCache(recipient *models.Recipient) {
	if !rm.config.EnableCache || recipient == nil {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if len(rm.cache) >= rm.config.CacheSize {
		for id := range rm.cache {
			delete(rm.cache, id)
			break
		}
	}

	rm.cache[recipient.ID] = recipient
}

func (rm *RecipientManager) getFromCache(id string) *models.Recipient {
	if !rm.config.EnableCache {
		return nil
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.cache[id]
}

func (rm *RecipientManager) removeFromCache(id string) {
	if !rm.config.EnableCache {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	delete(rm.cache, id)
}

func (rm *RecipientManager) updateStats(recipient *models.Recipient, operation string) {
	rm.stats.mu.Lock()
	defer rm.stats.mu.Unlock()

	switch operation {
	case "add":
		rm.stats.TotalRecipients++
		rm.stats.PendingRecipients++
	case "sent":
		rm.stats.SentRecipients++
		rm.stats.PendingRecipients--
	case "failed":
		rm.stats.FailedRecipients++
		rm.stats.PendingRecipients--
	}

	if _, exists := rm.stats.ByCampaign[recipient.CampaignID]; !exists {
		rm.stats.ByCampaign[recipient.CampaignID] = &CampaignRecipientStats{
			CampaignID: recipient.CampaignID,
		}
	}

	rm.stats.LastUpdated = time.Now()
}

func (rm *RecipientManager) ValidateEmail(email string) error {
	return rm.validator.ValidateEmail(email)
}
