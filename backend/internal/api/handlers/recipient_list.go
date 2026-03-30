package handlers

import (
        "database/sql"
        "encoding/json"
        "net/http"
        "strings"
        "time"

        "github.com/gorilla/mux"

        "email-campaign-system/pkg/errors"
        "email-campaign-system/pkg/logger"
)

type RecipientListHandler struct {
        db     *sql.DB
        logger logger.Logger
}

func NewRecipientListHandler(db *sql.DB, log logger.Logger) *RecipientListHandler {
        return &RecipientListHandler{db: db, logger: log}
}

type RecipientList struct {
        ID             string    `json:"id"`
        Name           string    `json:"name"`
        Description    string    `json:"description"`
        RecipientCount int       `json:"recipient_count"`
        CreatedAt      time.Time `json:"created_at"`
        UpdatedAt      time.Time `json:"updated_at"`
}

type CreateRecipientListRequest struct {
        Name        string `json:"name"`
        Description string `json:"description"`
}

func (h *RecipientListHandler) ListRecipientLists(w http.ResponseWriter, r *http.Request) {
        rows, err := h.db.QueryContext(r.Context(), `
                SELECT rl.id, rl.name, rl.description, 
                        COALESCE((SELECT COUNT(*) FROM recipients WHERE recipient_list_id = rl.id), 0) as recipient_count,
                        rl.created_at, rl.updated_at
                FROM recipient_lists rl
                ORDER BY rl.created_at DESC
        `)
        if err != nil {
                h.respondError(w, errors.Internal("Failed to list recipient lists"))
                return
        }
        defer rows.Close()

        lists := make([]RecipientList, 0)
        for rows.Next() {
                var rl RecipientList
                if err := rows.Scan(&rl.ID, &rl.Name, &rl.Description, &rl.RecipientCount, &rl.CreatedAt, &rl.UpdatedAt); err != nil {
                        continue
                }
                lists = append(lists, rl)
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "recipient_lists": lists,
                "total":           len(lists),
        })
}

func (h *RecipientListHandler) CreateRecipientList(w http.ResponseWriter, r *http.Request) {
        var req CreateRecipientListRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                h.respondError(w, errors.BadRequest("Invalid request body"))
                return
        }

        if req.Name == "" {
                h.respondError(w, errors.BadRequest("Name is required"))
                return
        }

        var rl RecipientList
        err := h.db.QueryRowContext(r.Context(), `
                INSERT INTO recipient_lists (name, description) VALUES ($1, $2)
                RETURNING id, name, description, recipient_count, created_at, updated_at
        `, req.Name, req.Description).Scan(&rl.ID, &rl.Name, &rl.Description, &rl.RecipientCount, &rl.CreatedAt, &rl.UpdatedAt)

        if err != nil {
                h.logger.Error("failed to create recipient list", logger.Error(err))
                h.respondError(w, errors.Internal("Failed to create recipient list"))
                return
        }

        h.respondJSON(w, http.StatusCreated, rl)
}

func (h *RecipientListHandler) GetRecipientList(w http.ResponseWriter, r *http.Request) {
        id := mux.Vars(r)["id"]

        var rl RecipientList
        err := h.db.QueryRowContext(r.Context(), `
                SELECT rl.id, rl.name, rl.description,
                        COALESCE((SELECT COUNT(*) FROM recipients WHERE recipient_list_id = rl.id), 0) as recipient_count,
                        rl.created_at, rl.updated_at
                FROM recipient_lists rl WHERE rl.id = $1
        `, id).Scan(&rl.ID, &rl.Name, &rl.Description, &rl.RecipientCount, &rl.CreatedAt, &rl.UpdatedAt)

        if err == sql.ErrNoRows {
                h.respondError(w, errors.NotFound("recipient_list", id))
                return
        }
        if err != nil {
                h.respondError(w, errors.Internal("Failed to get recipient list"))
                return
        }

        h.respondJSON(w, http.StatusOK, rl)
}

func (h *RecipientListHandler) DeleteRecipientList(w http.ResponseWriter, r *http.Request) {
        id := mux.Vars(r)["id"]

        _, err := h.db.ExecContext(r.Context(), `UPDATE recipients SET recipient_list_id = NULL WHERE recipient_list_id = $1`, id)
        if err != nil {
                h.respondError(w, errors.Internal("Failed to unlink recipients"))
                return
        }

        result, err := h.db.ExecContext(r.Context(), `DELETE FROM recipient_lists WHERE id = $1`, id)
        if err != nil {
                h.respondError(w, errors.Internal("Failed to delete recipient list"))
                return
        }

        rowsAffected, _ := result.RowsAffected()
        if rowsAffected == 0 {
                h.respondError(w, errors.NotFound("recipient_list", id))
                return
        }

        h.respondJSON(w, http.StatusOK, map[string]string{"message": "Recipient list deleted"})
}

func (h *RecipientListHandler) GetListRecipients(w http.ResponseWriter, r *http.Request) {
        id := mux.Vars(r)["id"]

        rows, err := h.db.QueryContext(r.Context(), `
                SELECT id, email, COALESCE(first_name, '') as first_name, COALESCE(last_name, '') as last_name, 
                        COALESCE(status, 'pending') as status, created_at
                FROM recipients WHERE recipient_list_id = $1
                ORDER BY created_at DESC LIMIT 500
        `, id)
        if err != nil {
                h.respondError(w, errors.Internal("Failed to get recipients"))
                return
        }
        defer rows.Close()

        type ListRecipient struct {
                ID        string    `json:"id"`
                Email     string    `json:"email"`
                FirstName string    `json:"first_name"`
                LastName  string    `json:"last_name"`
                Status    string    `json:"status"`
                CreatedAt time.Time `json:"created_at"`
        }

        recipients := make([]ListRecipient, 0)
        for rows.Next() {
                var rec ListRecipient
                if err := rows.Scan(&rec.ID, &rec.Email, &rec.FirstName, &rec.LastName, &rec.Status, &rec.CreatedAt); err != nil {
                        continue
                }
                recipients = append(recipients, rec)
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "recipients": recipients,
                "total":      len(recipients),
        })
}

func (h *RecipientListHandler) AddRecipientToList(w http.ResponseWriter, r *http.Request) {
        listID := mux.Vars(r)["id"]

        var exists bool
        err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM recipient_lists WHERE id = $1)`, listID).Scan(&exists)
        if err != nil || !exists {
                h.respondError(w, errors.NotFound("recipient_list", listID))
                return
        }

        var req struct {
                Email     string `json:"email"`
                FirstName string `json:"first_name"`
                LastName  string `json:"last_name"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                h.respondError(w, errors.BadRequest("Invalid request body"))
                return
        }
        if req.Email == "" {
                h.respondError(w, errors.BadRequest("Email is required"))
                return
        }
        req.Email = strings.ToLower(strings.TrimSpace(req.Email))

        // Reject duplicate email within the same list.
        var alreadyExists bool
        err = h.db.QueryRowContext(r.Context(),
                `SELECT EXISTS(SELECT 1 FROM recipients WHERE recipient_list_id = $1 AND LOWER(email) = $2)`,
                listID, req.Email).Scan(&alreadyExists)
        if err != nil {
                h.logger.Error("failed to check duplicate recipient", logger.Error(err))
                h.respondError(w, errors.Internal("Failed to check for duplicate"))
                return
        }
        if alreadyExists {
                h.respondError(w, errors.Conflict("Email already exists in this list: "+req.Email))
                return
        }

        var recID string
        err = h.db.QueryRowContext(r.Context(), `
                INSERT INTO recipients (email, first_name, last_name, recipient_list_id, status, is_valid, created_at, updated_at)
                VALUES ($1, $2, $3, $4, 'pending', true, NOW(), NOW())
                RETURNING id
        `, req.Email, req.FirstName, req.LastName, listID).Scan(&recID)

        if err != nil {
                h.logger.Error("failed to add recipient", logger.Error(err))
                h.respondError(w, errors.Internal("Failed to add recipient"))
                return
        }

        h.respondJSON(w, http.StatusCreated, map[string]string{"id": recID, "email": req.Email})
}

func (h *RecipientListHandler) ImportToList(w http.ResponseWriter, r *http.Request) {
        listID := mux.Vars(r)["id"]

        var listExists bool
        err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM recipient_lists WHERE id = $1)`, listID).Scan(&listExists)
        if err != nil || !listExists {
                h.respondError(w, errors.NotFound("recipient_list", listID))
                return
        }

        var req struct {
                Emails []string `json:"emails"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                h.respondError(w, errors.BadRequest("Invalid request body"))
                return
        }

        // Load all existing emails for this list to deduplicate efficiently.
        existing := make(map[string]bool)
        rows, err := h.db.QueryContext(r.Context(), `SELECT LOWER(email) FROM recipients WHERE recipient_list_id = $1`, listID)
        if err == nil {
                defer rows.Close()
                for rows.Next() {
                        var em string
                        if rows.Scan(&em) == nil {
                                existing[em] = true
                        }
                }
        }

        successful := 0
        failed := 0
        skipped := 0
        for _, line := range req.Emails {
                line = trimWhitespace(line)
                if line == "" {
                        continue
                }
                email, firstName, lastName := parseRecipientLine(line)
                if email == "" || !strings.Contains(email, "@") {
                        failed++
                        continue
                }
                email = strings.ToLower(strings.TrimSpace(email))
                // Skip if already in the list (pre-loaded or added earlier this import).
                if existing[email] {
                        skipped++
                        continue
                }
                existing[email] = true // mark to avoid intra-batch duplicates

                _, execErr := h.db.ExecContext(r.Context(), `
                        INSERT INTO recipients (email, first_name, last_name, recipient_list_id, status, is_valid, created_at, updated_at)
                        VALUES ($1, $2, $3, $4, 'pending', true, NOW(), NOW())
                `, email, firstName, lastName, listID)
                if execErr != nil {
                        failed++
                } else {
                        successful++
                }
        }

        h.respondJSON(w, http.StatusOK, map[string]interface{}{
                "total":      len(req.Emails),
                "successful": successful,
                "failed":     failed,
                "skipped":    skipped,
        })
}

func parseRecipientLine(line string) (email, firstName, lastName string) {
        line = trimWhitespace(line)
        if idx := strings.Index(line, "<"); idx >= 0 {
                if end := strings.Index(line, ">"); end > idx {
                        email = trimWhitespace(line[idx+1 : end])
                        namePart := trimWhitespace(line[:idx])
                        namePart = strings.Trim(namePart, "\"'")
                        parts := strings.Fields(namePart)
                        if len(parts) >= 2 {
                                firstName = parts[0]
                                lastName = strings.Join(parts[1:], " ")
                        } else if len(parts) == 1 {
                                firstName = parts[0]
                        }
                        return
                }
        }
        if strings.Contains(line, ",") {
                parts := strings.SplitN(line, ",", 3)
                email = trimWhitespace(parts[0])
                if len(parts) >= 2 {
                        firstName = trimWhitespace(parts[1])
                }
                if len(parts) >= 3 {
                        lastName = trimWhitespace(parts[2])
                }
                return
        }
        email = line
        return
}

func trimWhitespace(s string) string {
        start := 0
        end := len(s)
        for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
                start++
        }
        for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
                end--
        }
        return s[start:end]
}

func (h *RecipientListHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(status)
        json.NewEncoder(w).Encode(data)
}

func (h *RecipientListHandler) respondError(w http.ResponseWriter, err error) {
        status := http.StatusInternalServerError
        message := err.Error()

        if e, ok := err.(*errors.Error); ok {
                status = e.StatusCode
                // Use Details (the specific message) when present, fall back to Message.
                if e.Details != "" {
                        message = e.Details
                } else {
                        message = e.Message
                }
        }

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(status)
        json.NewEncoder(w).Encode(map[string]string{"error": message})
}
