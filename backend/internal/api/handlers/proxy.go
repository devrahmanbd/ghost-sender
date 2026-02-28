package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"email-campaign-system/internal/api/websocket"
	"email-campaign-system/internal/core/proxy"
	"email-campaign-system/internal/storage/repository"
	"email-campaign-system/pkg/errors"
	"email-campaign-system/pkg/logger"
	"email-campaign-system/pkg/validator"
)

type ProxyHandler struct {
	proxyManager *proxy.ProxyManager
	wsHub        *websocket.Hub
	logger       logger.Logger
	validator    *validator.Validator
}

func NewProxyHandler(
	proxyManager *proxy.ProxyManager,
	wsHub *websocket.Hub,
	log logger.Logger,
	validator *validator.Validator,
) *ProxyHandler {
	return &ProxyHandler{
		proxyManager: proxyManager,
		wsHub:        wsHub,
		logger:       log,
		validator:    validator,
	}
}

type CreateProxyRequest struct {
	Host     string `json:"host" validate:"required"`
	Port     int    `json:"port" validate:"required,min=1,max=65535"`
	Type     string `json:"type" validate:"required,oneof=http https socks5"`
	Username string `json:"username"`
	Password string `json:"password"`
	IsActive bool   `json:"is_active"`
}

type UpdateProxyRequest struct {
	Host     *string `json:"host"`
	Port     *int    `json:"port" validate:"omitempty,min=1,max=65535"`
	Type     *string `json:"type" validate:"omitempty,oneof=http https socks5"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	IsActive *bool   `json:"is_active"`
}

type ProxyResponse struct {
	ID                  string     `json:"id"`
	Host                string     `json:"host"`
	Port                int        `json:"port"`
	Type                string     `json:"type"`
	Username            string     `json:"username"`
	HasAuth             bool       `json:"has_auth"`
	IsActive            bool       `json:"is_active"`
	HealthStatus        string     `json:"health_status"`
	HealthScore         float64    `json:"health_score"`
	LastCheckedAt       *time.Time `json:"last_checked_at"`
	SuccessCount        int64      `json:"success_count"`
	FailureCount        int64      `json:"failure_count"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type TestProxyResponse struct {
	Success      bool    `json:"success"`
	ResponseTime int     `json:"response_time_ms"`
	Message      string  `json:"message,omitempty"`
	Error        string  `json:"error,omitempty"`
}

type ImportProxiesResponse struct {
	Total      int             `json:"total"`
	Successful int             `json:"successful"`
	Failed     int             `json:"failed"`
	Errors     []string        `json:"errors"`
	Proxies    []ProxyResponse `json:"proxies"`
}

func (h *ProxyHandler) CreateProxy(w http.ResponseWriter, r *http.Request) {

	var req CreateProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	// Create repository.Proxy
	newProxy := &repository.Proxy{
		ID:       generateID(), // You'll need to implement this
		Host:     req.Host,
		Port:     req.Port,
		Type:     req.Type,
		Username: req.Username,
		Password: req.Password,
		IsActive: req.IsActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.proxyManager.Add(newProxy); err != nil {
		h.logger.Error("Failed to create proxy", logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Proxy created successfully", logger.String("proxy_id", newProxy.ID), logger.String("host", newProxy.Host))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "proxy_created",
		Data: h.marshalJSON(newProxy),
	})

	h.respondJSON(w, http.StatusCreated, h.toProxyResponse(newProxy))
}

func (h *ProxyHandler) ListProxies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()
	proxyType := query.Get("type")
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("page_size"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filter := &repository.ProxyFilter{
		Types: []string{proxyType},
		Limit: pageSize,
		Offset: (page - 1) * pageSize,
	}

	proxies, total, err := h.proxyManager.List(ctx, filter)
	if err != nil {
		h.logger.Error("Failed to list proxies", logger.Error(err))
		h.respondError(w, err)
		return
	}

	response := make([]ProxyResponse, len(proxies))
	for i, prx := range proxies {
		response[i] = h.toProxyResponse(prx)
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"proxies":     response,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + pageSize - 1) / pageSize,
	})
}

func (h *ProxyHandler) GetProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid proxy ID"))
		return
	}

	prx, err := h.proxyManager.GetByID(ctx, id)
	if err != nil {
		h.logger.Error("Failed to get proxy", logger.String("proxy_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, h.toProxyResponse(prx))
}

func (h *ProxyHandler) UpdateProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid proxy ID"))
		return
	}

	var req UpdateProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	// Get existing proxy
	existingProxy, err := h.proxyManager.GetByID(ctx, id)
	if err != nil {
		h.logger.Error("Failed to get proxy", logger.String("proxy_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	// Update fields
	if req.Host != nil {
		existingProxy.Host = *req.Host
	}
	if req.Port != nil {
		existingProxy.Port = *req.Port
	}
	if req.Type != nil {
		existingProxy.Type = *req.Type
	}
	if req.Username != nil {
		existingProxy.Username = *req.Username
	}
	if req.Password != nil {
		existingProxy.Password = *req.Password
	}
	if req.IsActive != nil {
		existingProxy.IsActive = *req.IsActive
	}
	existingProxy.UpdatedAt = time.Now()

	if err := h.proxyManager.Update(existingProxy); err != nil {
		h.logger.Error("Failed to update proxy", logger.String("proxy_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Proxy updated successfully", logger.String("proxy_id", existingProxy.ID))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "proxy_updated",
		Data: h.marshalJSON(existingProxy),
	})

	h.respondJSON(w, http.StatusOK, h.toProxyResponse(existingProxy))
}

func (h *ProxyHandler) DeleteProxy(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid proxy ID"))
		return
	}

	if err := h.proxyManager.Remove(id); err != nil {
		h.logger.Error("Failed to delete proxy", logger.String("proxy_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.logger.Info("Proxy deleted successfully", logger.String("proxy_id", id))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "proxy_deleted",
		Data: h.marshalJSON(map[string]interface{}{"id": id}),
	})

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Proxy deleted successfully",
	})
}

func (h *ProxyHandler) TestProxy(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid proxy ID"))
		return
	}

	h.logger.Info("Testing proxy", logger.String("proxy_id", id))

	health, err := h.proxyManager.Test(id)
	if err != nil {
		h.logger.Error("Proxy test failed", logger.String("proxy_id", id), logger.Error(err))
		h.respondJSON(w, http.StatusOK, TestProxyResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	h.logger.Info("Proxy test successful", logger.String("proxy_id", id))

	response := TestProxyResponse{
		Success: health.Status == proxy.HealthStatusHealthy,
		Message: health.Message,
	}

	h.respondJSON(w, http.StatusOK, response)
}

func (h *ProxyHandler) BulkTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids" validate:"required,min=1"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, errors.BadRequest("Invalid request body"))
		return
	}

	if err := h.validator.Validate(req); err != nil {
		h.respondError(w, errors.ValidationError("validation", []string{err.Error()}))
		return
	}

	results := make([]map[string]interface{}, 0, len(req.IDs))
	
	for _, id := range req.IDs {
		health, err := h.proxyManager.Test(id)
		result := map[string]interface{}{
			"id":      id,
			"success": false,
		}
		
		if err != nil {
			result["error"] = err.Error()
		} else {
			result["success"] = health.Status == proxy.HealthStatusHealthy
			result["status"] = string(health.Status)
			result["message"] = health.Message
		}
		
		results = append(results, result)
	}

	h.logger.Info("Bulk proxy test completed", logger.Int("total", len(req.IDs)))

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"total":   len(req.IDs),
	})
}

func (h *ProxyHandler) GetProxyHealth(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid proxy ID"))
		return
	}

	health, err := h.proxyManager.Test(id)
	if err != nil {
		h.logger.Error("Failed to get proxy health", logger.String("proxy_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, health)
}

func (h *ProxyHandler) GetProxyStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	id := vars["id"]
	if id == "" {
		h.respondError(w, errors.BadRequest("Invalid proxy ID"))
		return
	}

	stats, err := h.proxyManager.GetProxyStats(id)
	if err != nil {
		h.logger.Error("Failed to get proxy stats", logger.String("proxy_id", id), logger.Error(err))
		h.respondError(w, err)
		return
	}

	h.respondJSON(w, http.StatusOK, stats)
}

func (h *ProxyHandler) ImportProxies(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.respondError(w, errors.BadRequest("Failed to parse multipart form"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.respondError(w, errors.BadRequest("No file uploaded"))
		return
	}
	defer file.Close()

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(header.Filename), "."))
	if ext != "txt" && ext != "csv" {
		h.respondError(w, errors.BadRequest("Only TXT and CSV files are supported"))
		return
	}

	content, err := io.ReadAll(file)
	if err != nil {
		h.respondError(w, errors.BadRequest("Failed to read file"))
		return
	}
	lines := strings.Split(string(content), "\n")
	total := 0
	successful := 0
	failed := 0
	errorsList := []string{}
	importedProxies := []*repository.Proxy{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		total++
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			failed++
			errorsList = append(errorsList, "Invalid format: "+line)
			continue
		}

		port, err := strconv.Atoi(parts[1])
		if err != nil {
			failed++
			errorsList = append(errorsList, "Invalid port: "+line)
			continue
		}

		newProxy := &repository.Proxy{
			ID:        generateID(),
			Host:      parts[0],
			Port:      port,
			Type:      "http", // Default type
			IsActive:  true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if len(parts) >= 4 {
			newProxy.Username = parts[2]
			newProxy.Password = parts[3]
		}

		if err := h.proxyManager.Add(newProxy); err != nil {
			failed++
			errorsList = append(errorsList, "Failed to add: "+line)
		} else {
			successful++
			importedProxies = append(importedProxies, newProxy)
		}
	}

	h.logger.Info("Proxies imported successfully",
		logger.String("filename", header.Filename),
		logger.Int("total", total),
		logger.Int("successful", successful),
		logger.Int("failed", failed))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "proxies_imported",
		Data: h.marshalJSON(map[string]interface{}{
			"total":      total,
			"successful": successful,
			"failed":     failed,
		}),
	})

	response := ImportProxiesResponse{
		Total:      total,
		Successful: successful,
		Failed:     failed,
		Errors:     errorsList,
		Proxies:    make([]ProxyResponse, len(importedProxies)),
	}

	for i, prx := range importedProxies {
		response.Proxies[i] = h.toProxyResponse(prx)
	}

	h.respondJSON(w, http.StatusOK, response)
}

func (h *ProxyHandler) CheckHealth(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Running health check on all proxies")

	// Get all proxies
	allProxies := h.proxyManager.GetAll()
	
	healthy := 0
	unhealthy := 0
	results := make([]map[string]interface{}, 0, len(allProxies))

	for _, prx := range allProxies {
		health, err := h.proxyManager.Test(prx.ID)
		
		result := map[string]interface{}{
			"id":   prx.ID,
			"host": prx.Host,
			"port": prx.Port,
		}

		if err != nil || health.Status != proxy.HealthStatusHealthy {
			unhealthy++
			result["healthy"] = false
			if err != nil {
				result["error"] = err.Error()
			}
		} else {
			healthy++
			result["healthy"] = true
		}

		results = append(results, result)
	}

	h.logger.Info("Health check completed",
		logger.Int("total", len(results)),
		logger.Int("healthy", healthy),
		logger.Int("unhealthy", unhealthy))

	h.wsHub.Broadcast(&websocket.Message{
		Type: "proxy_health_checked",
		Data: h.marshalJSON(map[string]interface{}{
			"total":     len(results),
			"healthy":   healthy,
			"unhealthy": unhealthy,
		}),
	})

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"total":     len(results),
		"healthy":   healthy,
		"unhealthy": unhealthy,
		"results":   results,
	})
}

func (h *ProxyHandler) GetRotationStats(w http.ResponseWriter, r *http.Request) {
	stats := h.proxyManager.GetStats()

	h.respondJSON(w, http.StatusOK, &stats)
}


func (h *ProxyHandler) toProxyResponse(prx *repository.Proxy) ProxyResponse {
	// Get proxy stats
	entry, _ := h.proxyManager.GetProxyStats(prx.ID)
	
	response := ProxyResponse{
		ID:                  prx.ID,
		Host:                prx.Host,
		Port:                prx.Port,
		Type:                prx.Type,
		Username:            prx.Username,
		HasAuth:             prx.Username != "",
		IsActive:            prx.IsActive,
		HealthStatus:        string(proxy.HealthStatusUnknown),
		SuccessCount:        0,
		FailureCount:        0,
		ConsecutiveFailures: 0,
		CreatedAt:           prx.CreatedAt,
		UpdatedAt:           prx.UpdatedAt,
	}

	if entry != nil {
		response.SuccessCount = entry.SuccessCount
		response.FailureCount = entry.FailureCount
		
		if entry.Health != nil {
			response.HealthStatus = string(entry.Health.Status)
			if !entry.Health.CheckedAt.IsZero() {
				response.LastCheckedAt = &entry.Health.CheckedAt
			}
		}
	}

	return response
}

func (h *ProxyHandler) marshalJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}

func (h *ProxyHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", logger.Error(err))
	}
}

func (h *ProxyHandler) respondError(w http.ResponseWriter, err error) {
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
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   message,
		"status":  status,
		"success": false,
	}); err != nil {
		h.logger.Error("Failed to encode error response", logger.Error(err))
	}
}

// Helper function to generate unique IDs
func generateID() string {
	return fmt.Sprintf("proxy_%d", time.Now().UnixNano())
}
