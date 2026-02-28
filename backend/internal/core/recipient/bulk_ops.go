package recipient

import (
    "context"
    "errors"
    "fmt"
    "strings"
    "time"

    "email-campaign-system/internal/models"
    "email-campaign-system/internal/storage/repository"
    "email-campaign-system/pkg/logger"
)

var (
    ErrInvalidCount      = errors.New("invalid count parameter")
    ErrInvalidRange      = errors.New("invalid range parameters")
    ErrNoRecipientsFound = errors.New("no recipients found for operation")
    ErrOperationFailed   = errors.New("bulk operation failed")
)

type BulkOperations struct {
    repo   repository.RecipientRepository
    logger logger.Logger
    config *BulkOpsConfig
}

type BulkOpsConfig struct {
    BatchSize          int
    MaxDeleteCount     int
    EnableTransactions bool
    EnableSafeMode     bool
    ConfirmRequired    bool
}

type BulkDeleteOperation struct {
    OperationType BulkOperationType
    CampaignID    string
    Count         int
    EmailBefore   string
    EmailAfter    string
    Status        models.RecipientStatus
    IDs           []string
    DryRun        bool
}

type BulkOperationType string

const (
    OpDeleteFirst    BulkOperationType = "delete_first"
    OpDeleteLast     BulkOperationType = "delete_last"
    OpDeleteBefore   BulkOperationType = "delete_before"
    OpDeleteAfter    BulkOperationType = "delete_after"
    OpDeleteByStatus BulkOperationType = "delete_by_status"
    OpDeleteByIDs    BulkOperationType = "delete_by_ids"
    OpDeleteAll      BulkOperationType = "delete_all"
    OpDeleteRange    BulkOperationType = "delete_range"
)

type BulkOperationResult struct {
    OperationType  BulkOperationType
    TotalProcessed int
    DeletedCount   int
    FailedCount    int
    Errors         []error
    AffectedIDs    []string
    Duration       time.Duration
    StartedAt      time.Time
    CompletedAt    time.Time
    DryRun         bool
}

func NewBulkOperations(repo repository.RecipientRepository, logger logger.Logger) *BulkOperations {
    return &BulkOperations{
        repo:   repo,
        logger: logger,
        config: DefaultBulkOpsConfig(),
    }
}

func DefaultBulkOpsConfig() *BulkOpsConfig {
    return &BulkOpsConfig{
        BatchSize:          1000,
        MaxDeleteCount:     100000,
        EnableTransactions: true,
        EnableSafeMode:     true,
        ConfirmRequired:    false,
    }
}

// Helper function to convert repository.Recipient to models.Recipient
func convertToModelsRecipients(repoRecipients []*repository.Recipient) []*models.Recipient {
    result := make([]*models.Recipient, len(repoRecipients))
    for i, r := range repoRecipients {
        result[i] = &models.Recipient{
            ID:        r.ID,
            Email:     r.Email,
            Status:    models.RecipientStatus(r.Status),
            CreatedAt: r.CreatedAt,
            UpdatedAt: r.UpdatedAt,
        }
    }
    return result
}

func (bo *BulkOperations) Execute(ctx context.Context, op *BulkDeleteOperation) (*BulkOperationResult, error) {
    startTime := time.Now()

    result := &BulkOperationResult{
        OperationType: op.OperationType,
        StartedAt:     startTime,
        DryRun:        op.DryRun,
        Errors:        make([]error, 0),
        AffectedIDs:   make([]string, 0),
    }

    if err := bo.validateOperation(op); err != nil {
        return result, err
    }

    var err error
    switch op.OperationType {
    case OpDeleteFirst:
        err = bo.deleteFirst(ctx, op, result)
    case OpDeleteLast:
        err = bo.deleteLast(ctx, op, result)
    case OpDeleteBefore:
        err = bo.deleteBefore(ctx, op, result)
    case OpDeleteAfter:
        err = bo.deleteAfter(ctx, op, result)
    case OpDeleteByStatus:
        err = bo.deleteByStatus(ctx, op, result)
    case OpDeleteByIDs:
        err = bo.deleteByIDs(ctx, op, result)
    case OpDeleteAll:
        err = bo.deleteAll(ctx, op, result)
    case OpDeleteRange:
        err = bo.deleteRange(ctx, op, result)
    default:
        err = fmt.Errorf("unsupported operation type: %s", op.OperationType)
    }

    result.CompletedAt = time.Now()
    result.Duration = result.CompletedAt.Sub(result.StartedAt)

    if err != nil {
        result.Errors = append(result.Errors, err)
    }

    bo.logger.Info(fmt.Sprintf("bulk operation completed: type=%s, campaign_id=%s, deleted=%d, failed=%d, duration=%v",
        op.OperationType, op.CampaignID, result.DeletedCount, result.FailedCount, result.Duration))

    return result, err
}

func (bo *BulkOperations) deleteFirst(ctx context.Context, op *BulkDeleteOperation, result *BulkOperationResult) error {
    if op.Count <= 0 {
        return ErrInvalidCount
    }

    filter := &repository.RecipientFilter{
        ListIDs: []string{op.CampaignID},
        Limit:   op.Count,
    }

    recipients, _, err := bo.repo.List(ctx, filter)
    if err != nil {
        return err
    }

    if len(recipients) == 0 {
        return ErrNoRecipientsFound
    }

    return bo.deleteRecipients(ctx, convertToModelsRecipients(recipients), op.DryRun, result)
}

func (bo *BulkOperations) deleteLast(ctx context.Context, op *BulkDeleteOperation, result *BulkOperationResult) error {
    if op.Count <= 0 {
        return ErrInvalidCount
    }

    filter := &repository.RecipientFilter{
        ListIDs: []string{op.CampaignID},
    }

    recipients, _, err := bo.repo.List(ctx, filter)
    if err != nil {
        return err
    }

    if len(recipients) == 0 {
        return ErrNoRecipientsFound
    }

    count := op.Count
    if count > len(recipients) {
        count = len(recipients)
    }

    startIndex := len(recipients) - count
    toDelete := recipients[startIndex:]
    return bo.deleteRecipients(ctx, convertToModelsRecipients(toDelete), op.DryRun, result)
}

func (bo *BulkOperations) deleteBefore(ctx context.Context, op *BulkDeleteOperation, result *BulkOperationResult) error {
    if op.EmailBefore == "" {
        return errors.New("email_before parameter is required")
    }

    filter := &repository.RecipientFilter{
        ListIDs: []string{op.CampaignID},
    }

    recipients, _, err := bo.repo.List(ctx, filter)
    if err != nil {
        return err
    }

    if len(recipients) == 0 {
        return ErrNoRecipientsFound
    }

    targetIndex := -1
    for i, r := range recipients {
        if strings.EqualFold(r.Email, op.EmailBefore) {
            targetIndex = i
            break
        }
    }

    if targetIndex == -1 {
        return fmt.Errorf("email not found: %s", op.EmailBefore)
    }

    if targetIndex == 0 {
        return errors.New("no recipients before the specified email")
    }

    toDelete := recipients[:targetIndex]
    return bo.deleteRecipients(ctx, convertToModelsRecipients(toDelete), op.DryRun, result)
}

func (bo *BulkOperations) deleteAfter(ctx context.Context, op *BulkDeleteOperation, result *BulkOperationResult) error {
    if op.EmailAfter == "" {
        return errors.New("email_after parameter is required")
    }

    filter := &repository.RecipientFilter{
        ListIDs: []string{op.CampaignID},
    }

    recipients, _, err := bo.repo.List(ctx, filter)
    if err != nil {
        return err
    }

    if len(recipients) == 0 {
        return ErrNoRecipientsFound
    }

    targetIndex := -1
    for i, r := range recipients {
        if strings.EqualFold(r.Email, op.EmailAfter) {
            targetIndex = i
            break
        }
    }

    if targetIndex == -1 {
        return fmt.Errorf("email not found: %s", op.EmailAfter)
    }

    if targetIndex >= len(recipients)-1 {
        return errors.New("no recipients after the specified email")
    }

    toDelete := recipients[targetIndex+1:]
    return bo.deleteRecipients(ctx, convertToModelsRecipients(toDelete), op.DryRun, result)
}

func (bo *BulkOperations) deleteByStatus(ctx context.Context, op *BulkDeleteOperation, result *BulkOperationResult) error {
    filter := &repository.RecipientFilter{
        ListIDs: []string{op.CampaignID},
        Status:  []string{string(op.Status)},
    }

    recipients, _, err := bo.repo.List(ctx, filter)
    if err != nil {
        return err
    }

    if len(recipients) == 0 {
        return ErrNoRecipientsFound
    }

    return bo.deleteRecipients(ctx, convertToModelsRecipients(recipients), op.DryRun, result)
}

func (bo *BulkOperations) deleteByIDs(ctx context.Context, op *BulkDeleteOperation, result *BulkOperationResult) error {
    if len(op.IDs) == 0 {
        return errors.New("no IDs provided")
    }

    if !op.DryRun {
        deleted := 0
        failed := 0

        for i := 0; i < len(op.IDs); i += bo.config.BatchSize {
            end := i + bo.config.BatchSize
            if end > len(op.IDs) {
                end = len(op.IDs)
            }

            batch := op.IDs[i:end]

            // Delete each ID individually
            for _, id := range batch {
                err := bo.repo.Delete(ctx, id)
                if err != nil {
                    bo.logger.Error(fmt.Sprintf("delete failed for id %s: %v", id, err))
                    failed++
                    result.Errors = append(result.Errors, err)
                    continue
                }
                deleted++
                result.AffectedIDs = append(result.AffectedIDs, id)
            }
        }

        result.DeletedCount = deleted
        result.FailedCount = failed
        result.TotalProcessed = len(op.IDs)
    } else {
        result.TotalProcessed = len(op.IDs)
        result.DeletedCount = len(op.IDs)
        result.AffectedIDs = op.IDs
    }

    return nil
}

func (bo *BulkOperations) deleteAll(ctx context.Context, op *BulkDeleteOperation, result *BulkOperationResult) error {
    if bo.config.EnableSafeMode && !op.DryRun {
        return errors.New("delete_all operation requires dry_run=false and safe_mode=false")
    }

    filter := &repository.RecipientFilter{
        ListIDs: []string{op.CampaignID},
    }

    if !op.DryRun {
        recipients, _, err := bo.repo.List(ctx, filter)
        if err != nil {
            return err
        }

        for _, r := range recipients {
            if err := bo.repo.Delete(ctx, r.ID); err != nil {
                result.Errors = append(result.Errors, err)
                result.FailedCount++
            } else {
                result.DeletedCount++
                result.AffectedIDs = append(result.AffectedIDs, r.ID)
            }
        }
        result.TotalProcessed = len(recipients)
    } else {
        count, err := bo.repo.Count(ctx, filter)
        if err != nil {
            return err
        }

        result.TotalProcessed = int(count)
        result.DeletedCount = int(count)
    }

    return nil
}

func (bo *BulkOperations) deleteRange(ctx context.Context, op *BulkDeleteOperation, result *BulkOperationResult) error {
    if op.EmailBefore == "" || op.EmailAfter == "" {
        return ErrInvalidRange
    }

    filter := &repository.RecipientFilter{
        ListIDs: []string{op.CampaignID},
    }

    recipients, _, err := bo.repo.List(ctx, filter)
    if err != nil {
        return err
    }

    if len(recipients) == 0 {
        return ErrNoRecipientsFound
    }

    startIndex := -1
    endIndex := -1

    for i, r := range recipients {
        if strings.EqualFold(r.Email, op.EmailBefore) {
            startIndex = i
        }
        if strings.EqualFold(r.Email, op.EmailAfter) {
            endIndex = i
        }
    }

    if startIndex == -1 || endIndex == -1 {
        return errors.New("one or both range emails not found")
    }

    if startIndex >= endIndex {
        return errors.New("invalid range: start email must come before end email")
    }

    toDelete := recipients[startIndex : endIndex+1]
    return bo.deleteRecipients(ctx, convertToModelsRecipients(toDelete), op.DryRun, result)
}

func (bo *BulkOperations) deleteRecipients(ctx context.Context, recipients []*models.Recipient, dryRun bool, result *BulkOperationResult) error {
    result.TotalProcessed = len(recipients)

    if dryRun {
        for _, r := range recipients {
            result.AffectedIDs = append(result.AffectedIDs, r.ID)
        }
        result.DeletedCount = len(recipients)
        return nil
    }

    deleted := 0
    failed := 0

    for i := 0; i < len(recipients); i += bo.config.BatchSize {
        end := i + bo.config.BatchSize
        if end > len(recipients) {
            end = len(recipients)
        }

        batch := recipients[i:end]

        for _, r := range batch {
            err := bo.repo.Delete(ctx, r.ID)
            if err != nil {
                bo.logger.Error(fmt.Sprintf("delete failed for id %s: %v", r.ID, err))
                failed++
                result.Errors = append(result.Errors, err)
                continue
            }

            deleted++
            result.AffectedIDs = append(result.AffectedIDs, r.ID)
        }
    }

    result.DeletedCount = deleted
    result.FailedCount = failed

    return nil
}

func (bo *BulkOperations) validateOperation(op *BulkDeleteOperation) error {
    if op == nil {
        return errors.New("operation is nil")
    }

    if op.CampaignID == "" && op.OperationType != OpDeleteByIDs {
        return errors.New("campaign_id is required")
    }

    if op.Count > bo.config.MaxDeleteCount {
        return fmt.Errorf("count exceeds maximum: %d > %d", op.Count, bo.config.MaxDeleteCount)
    }

    return nil
}

func (bo *BulkOperations) DeleteFirstN(ctx context.Context, campaignID string, count int, dryRun bool) (*BulkOperationResult, error) {
    op := &BulkDeleteOperation{
        OperationType: OpDeleteFirst,
        CampaignID:    campaignID,
        Count:         count,
        DryRun:        dryRun,
    }
    return bo.Execute(ctx, op)
}

func (bo *BulkOperations) DeleteLastN(ctx context.Context, campaignID string, count int, dryRun bool) (*BulkOperationResult, error) {
    op := &BulkDeleteOperation{
        OperationType: OpDeleteLast,
        CampaignID:    campaignID,
        Count:         count,
        DryRun:        dryRun,
    }
    return bo.Execute(ctx, op)
}

func (bo *BulkOperations) DeleteBeforeEmail(ctx context.Context, campaignID, email string, dryRun bool) (*BulkOperationResult, error) {
    op := &BulkDeleteOperation{
        OperationType: OpDeleteBefore,
        CampaignID:    campaignID,
        EmailBefore:   email,
        DryRun:        dryRun,
    }
    return bo.Execute(ctx, op)
}

func (bo *BulkOperations) DeleteAfterEmail(ctx context.Context, campaignID, email string, dryRun bool) (*BulkOperationResult, error) {
    op := &BulkDeleteOperation{
        OperationType: OpDeleteAfter,
        CampaignID:    campaignID,
        EmailAfter:    email,
        DryRun:        dryRun,
    }
    return bo.Execute(ctx, op)
}

func (bo *BulkOperations) DeleteByStatus(ctx context.Context, campaignID string, status models.RecipientStatus, dryRun bool) (*BulkOperationResult, error) {
    op := &BulkDeleteOperation{
        OperationType: OpDeleteByStatus,
        CampaignID:    campaignID,
        Status:        status,
        DryRun:        dryRun,
    }
    return bo.Execute(ctx, op)
}

func (bo *BulkOperations) DeleteByIDs(ctx context.Context, ids []string, dryRun bool) (*BulkOperationResult, error) {
    op := &BulkDeleteOperation{
        OperationType: OpDeleteByIDs,
        IDs:           ids,
        DryRun:        dryRun,
    }
    return bo.Execute(ctx, op)
}

func (bo *BulkOperations) DeleteRange(ctx context.Context, campaignID, startEmail, endEmail string, dryRun bool) (*BulkOperationResult, error) {
    op := &BulkDeleteOperation{
        OperationType: OpDeleteRange,
        CampaignID:    campaignID,
        EmailBefore:   startEmail,
        EmailAfter:    endEmail,
        DryRun:        dryRun,
    }
    return bo.Execute(ctx, op)
}

func (bo *BulkOperations) GetOperationPreview(ctx context.Context, op *BulkDeleteOperation) (*BulkOperationResult, error) {
    op.DryRun = true
    return bo.Execute(ctx, op)
}

func (bo *BulkOperations) SetConfig(config *BulkOpsConfig) {
    bo.config = config
}

func (bo *BulkOperations) GetConfig() *BulkOpsConfig {
    return bo.config
}
