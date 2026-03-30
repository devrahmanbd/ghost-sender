package handlers

import (
        "context"
        "encoding/csv"
        "fmt"
        "os"
        stderrors "errors"
        "email-campaign-system/internal/core/recipient"
        "email-campaign-system/internal/models"
        pkgerrors "email-campaign-system/pkg/errors"
)

type recipientManagerAdapter struct {
        core *recipient.RecipientManager
}

func NewRecipientManagerAdapter(mgr *recipient.RecipientManager) RecipientManager {
        return &recipientManagerAdapter{core: mgr}
}

func (a *recipientManagerAdapter) Create(ctx context.Context, req *CreateRecipientReq) (*models.Recipient, error) {
        campaignID := ""
        if req.CampaignID != nil {
                campaignID = *req.CampaignID
        }
        rec := models.Recipient{
                Email:      req.Email,
                FirstName:  req.FirstName,
                LastName:   req.LastName,
                Tags:       req.Tags,
                CampaignID: campaignID,
                Status:     models.RecipientStatusPending,
                IsValid:    true,
        }

        if err := a.core.AddRecipient(ctx, &rec); err != nil {
                if stderrors.Is(err, recipient.ErrDuplicateRecipient) {
                        return nil, pkgerrors.Conflict("recipient already exists: " + req.Email)
                }
                return nil, err
        }

        created, err := a.core.GetByEmail(ctx, campaignID, req.Email)
        if err != nil {
                rec.IsValid = true
                return &rec, nil
        }

        if created.FirstName == "" {
                created.FirstName = req.FirstName
        }
        if created.LastName == "" {
                created.LastName = req.LastName
        }
        if len(created.Tags) == 0 {
                created.Tags = req.Tags
        }
        created.IsValid = true

        return created, nil
}



func (a *recipientManagerAdapter) List(ctx context.Context, opts *ListRecipientOptions) ([]*models.Recipient, int, error) {
    if opts == nil {
        opts = &ListRecipientOptions{Page: 1, PageSize: 100}
    }
    offset := 0
    if opts.Page > 1 {
        offset = (opts.Page - 1) * opts.PageSize
    }
    filter := &recipient.RecipientFilter{
        SearchTerm: opts.Search,
        Status:     models.RecipientStatus(opts.Status),
        Limit:      opts.PageSize,
        Offset:     offset,
        SortBy:     opts.SortBy,
        SortOrder:  opts.SortOrder,
    }
    if opts.CampaignID != nil {
        filter.CampaignID = *opts.CampaignID
    }
    recs, err := a.core.GetRecipients(ctx, filter)
    if err != nil {
        return nil, 0, err
    }
    total, err := a.core.CountRecipients(ctx, filter)
    if err != nil {
        return nil, 0, err
    }
    return recs, int(total), nil
}


func (a *recipientManagerAdapter) GetByID(ctx context.Context, id string) (*models.Recipient, error) {
        return a.core.GetRecipient(ctx, id)
}

// Update — UpdateRecipientReq fields are *string.
func (a *recipientManagerAdapter) Update(ctx context.Context, id string, req *UpdateRecipientReq) (*models.Recipient, error) {
        rec, err := a.core.GetRecipient(ctx, id)
        if err != nil {
                return nil, err
        }
        if req.Email != nil {
                rec.Email = *req.Email
        }
        if req.FirstName != nil {
                rec.FirstName = *req.FirstName
        }
        if req.LastName != nil {
                rec.LastName = *req.LastName
        }
        if req.Tags != nil {
                rec.Tags = req.Tags
        }
        if err := a.core.UpdateRecipient(ctx, rec); err != nil {
                return nil, err
        }
        return rec, nil
}

func (a *recipientManagerAdapter) Delete(ctx context.Context, id string) error {
        return a.core.DeleteRecipient(ctx, id)
}

// ImportFromFile — req.CampaignID is *string.
func (a *recipientManagerAdapter) ImportFromFile(ctx context.Context, req *ImportRecipientRequest) (*ImportRecipientResult, error) {
        tmpFile, err := os.CreateTemp("", "recipients-*"+req.FileType)
        if err != nil {
                return nil, fmt.Errorf("failed to create temp file: %w", err)
        }
        defer os.Remove(tmpFile.Name())
        if _, err := tmpFile.Write(req.Content); err != nil {
                tmpFile.Close()
                return nil, fmt.Errorf("failed to write temp file: %w", err)
        }
        tmpFile.Close()

        campaignID := ""
        if req.CampaignID != nil {
                campaignID = *req.CampaignID
        }
        added, err := a.core.ImportFromFile(ctx, campaignID, tmpFile.Name())
        if err != nil {
                return &ImportRecipientResult{Failed: 1, Errors: []string{err.Error()}}, err
        }
        return &ImportRecipientResult{Total: added, Successful: added}, nil
}
func (a *recipientManagerAdapter) ValidateEmail(_ context.Context, email string) (*EmailValidationResult, error) {
    if err := a.core.ValidateEmail(email); err != nil {
        return &EmailValidationResult{IsValid: false, Reason: err.Error()}, nil
    }
    return &EmailValidationResult{IsValid: true}, nil
}

func (a *recipientManagerAdapter) BulkDelete(ctx context.Context, ids []string) (int, error) {
        return a.core.DeleteRecipients(ctx, ids)
}

func (a *recipientManagerAdapter) DeleteFirst(ctx context.Context, campaignID string, count int) (int, error) {
        recs, err := a.core.GetRecipients(ctx, &recipient.RecipientFilter{ // FIX: & pointer
                CampaignID: campaignID,
                Limit:      count,
                SortBy:     "created_at",
                SortOrder:  "asc",
        })
        if err != nil {
                return 0, err
        }
        return a.bulkDeleteFromSlice(ctx, recs)
}

func (a *recipientManagerAdapter) DeleteLast(ctx context.Context, campaignID string, count int) (int, error) {
        recs, err := a.core.GetRecipients(ctx, &recipient.RecipientFilter{ // FIX: & pointer
                CampaignID: campaignID,
                Limit:      count,
                SortBy:     "created_at",
                SortOrder:  "desc",
        })
        if err != nil {
                return 0, err
        }
        return a.bulkDeleteFromSlice(ctx, recs)
}

func (a *recipientManagerAdapter) DeleteBefore(ctx context.Context, campaignID string, email string) (int, error) {
        recs, err := a.core.GetRecipients(ctx, &recipient.RecipientFilter{ // FIX: & pointer
                CampaignID: campaignID,
                Limit:      100000,
                SortBy:     "created_at",
                SortOrder:  "asc",
        })
        if err != nil {
                return 0, err
        }
        ids := make([]string, 0)
        for _, r := range recs {
                if r == nil {
                        continue
                }
                if r.Email == email {
                        break
                }
                ids = append(ids, r.ID)
        }
        if len(ids) == 0 {
                return 0, nil
        }
        return a.core.DeleteRecipients(ctx, ids)
}

func (a *recipientManagerAdapter) DeleteAfter(ctx context.Context, campaignID string, email string) (int, error) {
        recs, err := a.core.GetRecipients(ctx, &recipient.RecipientFilter{ // FIX: & pointer
                CampaignID: campaignID,
                Limit:      100000,
                SortBy:     "created_at",
                SortOrder:  "asc",
        })
        if err != nil {
                return 0, err
        }
        ids := make([]string, 0)
        found := false
        for _, r := range recs {
                if r == nil {
                        continue
                }
                if found {
                        ids = append(ids, r.ID)
                }
                if r.Email == email {
                        found = true
                }
        }
        if len(ids) == 0 {
                return 0, nil
        }
        return a.core.DeleteRecipients(ctx, ids)
}

func (a *recipientManagerAdapter) RemoveDuplicates(ctx context.Context, campaignID string) (int, error) {
        recs, err := a.core.GetRecipients(ctx, &recipient.RecipientFilter{ // FIX: & pointer
                CampaignID: campaignID,
                Limit:      100000,
        })
        if err != nil {
                return 0, err
        }
        seen := make(map[string]bool)
        dupIDs := make([]string, 0)
        for _, r := range recs {
                if r == nil {
                        continue
                }
                if seen[r.Email] {
                        dupIDs = append(dupIDs, r.ID)
                } else {
                        seen[r.Email] = true
                }
        }
        if len(dupIDs) == 0 {
                return 0, nil
        }
        return a.core.DeleteRecipients(ctx, dupIDs)
}
func (a *recipientManagerAdapter) GetStats(ctx context.Context, campaignID *string) (interface{}, error) {
    // ✅ nil campaignID = global stats across all campaigns
    if campaignID == nil || *campaignID == "" {
        return map[string]interface{}{
            "message": "Provide ?campaign_id=<uuid> for campaign-specific stats",
        }, nil
    }
    return a.core.GetStatistics(ctx, *campaignID)
}

func (a *recipientManagerAdapter) ExportToFile(ctx context.Context, campaignID *string, format string, filename string) error {
        cid := ""
        if campaignID != nil {
                cid = *campaignID
        }
        recs, err := a.core.GetRecipients(ctx, &recipient.RecipientFilter{ // FIX: & pointer
                CampaignID: cid,
                Limit:      100000,
        })
        if err != nil {
                return err
        }
        f, err := os.Create(filename)
        if err != nil {
                return fmt.Errorf("failed to create export file: %w", err)
        }
        defer f.Close()

        w := csv.NewWriter(f)
        defer w.Flush()
        _ = w.Write([]string{"id", "email", "first_name", "last_name", "status", "created_at"})
        for _, r := range recs {
                if r == nil {
                        continue
                }
                _ = w.Write([]string{r.ID, r.Email, r.FirstName, r.LastName, string(r.Status), r.CreatedAt.String()})
        }
        return nil
}

// bulkDeleteFromSlice extracts IDs from a recipient slice and deletes them.
func (a *recipientManagerAdapter) bulkDeleteFromSlice(ctx context.Context, recs []*models.Recipient) (int, error) {
        ids := make([]string, 0, len(recs))
        for _, r := range recs {
                if r != nil {
                        ids = append(ids, r.ID)
                }
        }
        if len(ids) == 0 {
                return 0, nil
        }
        return a.core.DeleteRecipients(ctx, ids)
}
