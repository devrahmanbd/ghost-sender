package errors

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ErrorResponse struct {
	Error     string                 `json:"error" xml:"error"`
	Code      string                 `json:"code" xml:"code"`
	Message   string                 `json:"message" xml:"message"`
	Details   string                 `json:"details,omitempty" xml:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp" xml:"timestamp"`
	Path      string                 `json:"path,omitempty" xml:"path,omitempty"`
	RequestID string                 `json:"request_id,omitempty" xml:"request_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" xml:"metadata,omitempty"`
	Retryable bool                   `json:"retryable,omitempty" xml:"retryable,omitempty"`
}

type ValidationErrorResponse struct {
	Error      string                      `json:"error" xml:"error"`
	Code       string                      `json:"code" xml:"code"`
	Message    string                      `json:"message" xml:"message"`
	Timestamp  time.Time                   `json:"timestamp" xml:"timestamp"`
	Path       string                      `json:"path,omitempty" xml:"path,omitempty"`
	RequestID  string                      `json:"request_id,omitempty" xml:"request_id,omitempty"`
	Violations []ValidationViolation       `json:"violations" xml:"violations"`
	Metadata   map[string]interface{}      `json:"metadata,omitempty" xml:"metadata,omitempty"`
}

type ValidationViolation struct {
	Field   string `json:"field" xml:"field"`
	Message string `json:"message" xml:"message"`
	Value   string `json:"value,omitempty" xml:"value,omitempty"`
}

type ProblemDetail struct {
	Type     string                 `json:"type" xml:"type"`
	Title    string                 `json:"title" xml:"title"`
	Status   int                    `json:"status" xml:"status"`
	Detail   string                 `json:"detail,omitempty" xml:"detail,omitempty"`
	Instance string                 `json:"instance,omitempty" xml:"instance,omitempty"`
	Extra    map[string]interface{} `json:"-" xml:"-"`
}

func (p *ProblemDetail) MarshalJSON() ([]byte, error) {
	type Alias ProblemDetail
	base, err := json.Marshal((*Alias)(p))
	if err != nil {
		return nil, err
	}

	if len(p.Extra) == 0 {
		return base, nil
	}

	var baseMap map[string]interface{}
	if err := json.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}

	for k, v := range p.Extra {
		baseMap[k] = v
	}

	return json.Marshal(baseMap)
}

type Handler struct {
	logger       Logger
	debug        bool
	includeStack bool
	problemJSON  bool
}

type Logger interface {
	Error(ctx context.Context, msg string, fields map[string]interface{})
	Warn(ctx context.Context, msg string, fields map[string]interface{})
}

type HandlerOption func(*Handler)

func WithLogger(logger Logger) HandlerOption {
	return func(h *Handler) {
		h.logger = logger
	}
}

func WithDebug(debug bool) HandlerOption {
	return func(h *Handler) {
		h.debug = debug
	}
}

func WithStackTrace(include bool) HandlerOption {
	return func(h *Handler) {
		h.includeStack = include
	}
}

func WithProblemJSON(enable bool) HandlerOption {
	return func(h *Handler) {
		h.problemJSON = enable
	}
}

func NewHandler(opts ...HandlerOption) *Handler {
	h := &Handler{
		debug:        false,
		includeStack: false,
		problemJSON:  false,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	
	var customErr *Error
	if errors.As(err, &customErr) {
		h.handleCustomError(w, r, customErr)
		return
	}

	h.handleStandardError(w, r, err)
}

func (h *Handler) handleCustomError(w http.ResponseWriter, r *http.Request, err *Error) {
	ctx := r.Context()

	statusCode := err.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}

	if h.logger != nil {
		logLevel := h.getLogLevel(statusCode)
		fields := map[string]interface{}{
			"error_code":   err.Code,
			"status_code":  statusCode,
			"path":         r.URL.Path,
			"method":       r.Method,
			"request_id":   getRequestID(ctx),
			"metadata":     err.Metadata,
		}

		if err.Err != nil {
			fields["underlying_error"] = err.Err.Error()
		}

		if h.includeStack && len(err.Stack) > 0 {
			fields["stack"] = err.Stack
		}

		if logLevel == "error" {
			h.logger.Error(ctx, err.Message, fields)
		} else if logLevel == "warn" {
			h.logger.Warn(ctx, err.Message, fields)
		}
	}

	if h.problemJSON && acceptsProblemJSON(r) {
		h.writeProblemJSON(w, r, err, statusCode)
		return
	}

	if err.Code == ErrValidation {
		h.writeValidationError(w, r, err, statusCode)
		return
	}

	h.writeStandardError(w, r, err, statusCode)
}

func (h *Handler) handleStandardError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	
	statusCode := http.StatusInternalServerError
	code := ErrInternal
	message := "Internal server error"
	details := ""

	if h.debug {
		details = err.Error()
	}

	customErr := &Error{
		Code:       code,
		Message:    message,
		Details:    details,
		Err:        err,
		StatusCode: statusCode,
		Timestamp:  time.Now(),
		Retryable:  false,
	}

	if h.logger != nil {
		h.logger.Error(ctx, message, map[string]interface{}{
			"error":      err.Error(),
			"status":     statusCode,
			"path":       r.URL.Path,
			"method":     r.Method,
			"request_id": getRequestID(ctx),
		})
	}

	h.writeStandardError(w, r, customErr, statusCode)
}

func (h *Handler) writeStandardError(w http.ResponseWriter, r *http.Request, err *Error, statusCode int) {
	response := ErrorResponse{
		Error:     http.StatusText(statusCode),
		Code:      err.Code,
		Message:   err.Message,
		Details:   err.Details,
		Timestamp: err.Timestamp,
		Path:      r.URL.Path,
		RequestID: getRequestID(r.Context()),
		Metadata:  err.Metadata,
		Retryable: err.Retryable,
	}

	if !h.debug {
		if statusCode == http.StatusInternalServerError {
			response.Details = ""
			response.Metadata = nil
		}
	}

	h.writeResponse(w, r, response, statusCode)
}

func (h *Handler) writeValidationError(w http.ResponseWriter, r *http.Request, err *Error, statusCode int) {
	violations := h.extractViolations(err)

	response := ValidationErrorResponse{
		Error:      http.StatusText(statusCode),
		Code:       err.Code,
		Message:    err.Message,
		Timestamp:  err.Timestamp,
		Path:       r.URL.Path,
		RequestID:  getRequestID(r.Context()),
		Violations: violations,
		Metadata:   err.Metadata,
	}

	h.writeResponse(w, r, response, statusCode)
}

func (h *Handler) writeProblemJSON(w http.ResponseWriter, r *http.Request, err *Error, statusCode int) {
	problemType := fmt.Sprintf("https://httpstatuses.com/%d", statusCode)
	if err.Code != "" {
		problemType = fmt.Sprintf("/errors/%s", strings.ToLower(strings.ReplaceAll(err.Code, "_", "-")))
	}

	problem := ProblemDetail{
		Type:     problemType,
		Title:    err.Message,
		Status:   statusCode,
		Detail:   err.Details,
		Instance: r.URL.Path,
		Extra:    make(map[string]interface{}),
	}

	problem.Extra["code"] = err.Code
	problem.Extra["timestamp"] = err.Timestamp
	problem.Extra["request_id"] = getRequestID(r.Context())
	problem.Extra["retryable"] = err.Retryable

	if err.Metadata != nil && len(err.Metadata) > 0 {
		problem.Extra["metadata"] = err.Metadata
	}

	if h.includeStack && len(err.Stack) > 0 {
		problem.Extra["stack"] = err.Stack
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(problem)
}

func (h *Handler) writeResponse(w http.ResponseWriter, r *http.Request, response interface{}, statusCode int) {
	contentType := h.negotiateContentType(r)
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(statusCode)

	switch contentType {
	case "application/xml", "text/xml":
		xml.NewEncoder(w).Encode(response)
	default:
		json.NewEncoder(w).Encode(response)
	}
}

func (h *Handler) negotiateContentType(r *http.Request) string {
	accept := r.Header.Get("Accept")
	
	if strings.Contains(accept, "application/xml") || strings.Contains(accept, "text/xml") {
		return "application/xml"
	}

	return "application/json"
}

func (h *Handler) extractViolations(err *Error) []ValidationViolation {
	violations := []ValidationViolation{}

	if err.Metadata != nil {
		if field, ok := err.Metadata["field"].(string); ok {
			violation := ValidationViolation{
				Field:   field,
				Message: err.Details,
			}

			if value, ok := err.Metadata["value"].(string); ok {
				violation.Value = value
			}

			violations = append(violations, violation)
		}

		if viols, ok := err.Metadata["violations"].([]string); ok {
			if field, ok := err.Metadata["field"].(string); ok {
				for _, v := range viols {
					violations = append(violations, ValidationViolation{
						Field:   field,
						Message: v,
					})
				}
			}
		}

		if violsList, ok := err.Metadata["violations"].([]ValidationViolation); ok {
			violations = append(violations, violsList...)
		}
	}

	if len(violations) == 0 && err.Details != "" {
		violations = append(violations, ValidationViolation{
			Field:   "unknown",
			Message: err.Details,
		})
	}

	return violations
}

func (h *Handler) getLogLevel(statusCode int) string {
	if statusCode >= 500 {
		return "error"
	}
	if statusCode >= 400 {
		return "warn"
	}
	return "info"
}

func acceptsProblemJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/problem+json")
}

func getRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value("request_id").(string); ok {
		return reqID
	}
	if reqID, ok := ctx.Value("requestID").(string); ok {
		return reqID
	}
	return ""
}

func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	defaultHandler.Handle(w, r, err)
}

func HandleErrorWithStatus(w http.ResponseWriter, r *http.Request, err error, statusCode int) {
	var customErr *Error
	if errors.As(err, &customErr) {
		customErr.StatusCode = statusCode
		defaultHandler.Handle(w, r, customErr)
		return
	}

	customErr = &Error{
		Code:       GetCodeForStatus(statusCode),
		Message:    http.StatusText(statusCode),
		Details:    err.Error(),
		Err:        err,
		StatusCode: statusCode,
		Timestamp:  time.Now(),
		Retryable:  false,
	}

	defaultHandler.Handle(w, r, customErr)
}

func WriteJSON(w http.ResponseWriter, r *http.Request, err error) {
	var customErr *Error
	if !errors.As(err, &customErr) {
		customErr = FromStdError(err)
	}

	statusCode := customErr.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}

	response := ErrorResponse{
		Error:     http.StatusText(statusCode),
		Code:      customErr.Code,
		Message:   customErr.Message,
		Details:   customErr.Details,
		Timestamp: customErr.Timestamp,
		Path:      r.URL.Path,
		RequestID: getRequestID(r.Context()),
		Metadata:  customErr.Metadata,
		Retryable: customErr.Retryable,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func WriteValidationError(w http.ResponseWriter, r *http.Request, violations []ValidationViolation) {
	response := ValidationErrorResponse{
		Error:      "Bad Request",
		Code:       ErrValidation,
		Message:    "Validation failed",
		Timestamp:  time.Now(),
		Path:       r.URL.Path,
		RequestID:  getRequestID(r.Context()),
		Violations: violations,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response)
}

func WriteProblemDetail(w http.ResponseWriter, r *http.Request, problem ProblemDetail) {
	if problem.Status == 0 {
		problem.Status = http.StatusInternalServerError
	}

	if problem.Instance == "" {
		problem.Instance = r.URL.Path
	}

	if problem.Extra == nil {
		problem.Extra = make(map[string]interface{})
	}

	problem.Extra["request_id"] = getRequestID(r.Context())
	problem.Extra["timestamp"] = time.Now()

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	json.NewEncoder(w).Encode(problem)
}

func GetCodeForStatus(statusCode int) string {
	statusToCode := map[int]string{
		http.StatusBadRequest:          ErrBadRequest,
		http.StatusUnauthorized:        ErrUnauthorized,
		http.StatusForbidden:           ErrForbidden,
		http.StatusNotFound:            ErrNotFound,
		http.StatusConflict:            ErrConflict,
		http.StatusTooManyRequests:     ErrRateLimitExceeded,
		http.StatusInternalServerError: ErrInternal,
		http.StatusServiceUnavailable:  ErrServiceUnavailable,
		http.StatusGatewayTimeout:      ErrTimeout,
	}

	if code, ok := statusToCode[statusCode]; ok {
		return code
	}

	return ErrInternal
}

func Middleware(handler *Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ew := &errorWriter{
				ResponseWriter: w,
				handler:        handler,
				request:        r,
			}

			next.ServeHTTP(ew, r)
		})
	}
}

type errorWriter struct {
	http.ResponseWriter
	handler *Handler
	request *http.Request
	written bool
}

func (ew *errorWriter) WriteError(err error) {
	if ew.written {
		return
	}
	ew.written = true
	ew.handler.Handle(ew.ResponseWriter, ew.request, err)
}

func (ew *errorWriter) Write(b []byte) (int, error) {
	ew.written = true
	return ew.ResponseWriter.Write(b)
}

func (ew *errorWriter) WriteHeader(statusCode int) {
	ew.written = true
	ew.ResponseWriter.WriteHeader(statusCode)
}

var defaultHandler = NewHandler()

func SetDefaultHandler(handler *Handler) {
	defaultHandler = handler
}

func GetDefaultHandler() *Handler {
	return defaultHandler
}

func RecoverHandler(w http.ResponseWriter, r *http.Request, recovered interface{}) {
	err := Internal(fmt.Sprintf("panic recovered: %v", recovered))
	defaultHandler.Handle(w, r, err)
}

func BadRequestHandler(w http.ResponseWriter, r *http.Request, message string) {
	err := InvalidInput("request", message)
	defaultHandler.Handle(w, r, err)
}

func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	err := NotFound("resource", r.URL.Path)
	defaultHandler.Handle(w, r, err)
}

func MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	err := &Error{
		Code:       ErrMethodNotAllowed,
		Message:    "Method not allowed",
		Details:    fmt.Sprintf("Method %s is not allowed for this endpoint", r.Method),
		StatusCode: http.StatusMethodNotAllowed,
		Timestamp:  time.Now(),
	}
	defaultHandler.Handle(w, r, err)
}

func UnauthorizedHandler(w http.ResponseWriter, r *http.Request, message string) {
	err := Unauthorized(message)
	defaultHandler.Handle(w, r, err)
}

func ForbiddenHandler(w http.ResponseWriter, r *http.Request, message string) {
	err := Forbidden(message)
	defaultHandler.Handle(w, r, err)
}

