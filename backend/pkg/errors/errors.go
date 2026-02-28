package errors

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type Error struct {
	Code       string                 `json:"code"`
	Message    string                 `json:"message"`
	Details    string                 `json:"details,omitempty"`
	Err        error                  `json:"-"`
	StatusCode int                    `json:"-"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Stack      []StackFrame           `json:"stack,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Retryable  bool                   `json:"retryable"`
}

type StackFrame struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"function"`
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	if e.Details != "" {
		return fmt.Sprintf("%s: %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) WithMetadata(key string, value interface{}) *Error {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

func (e *Error) WithDetails(details string) *Error {
	e.Details = details
	return e
}

func (e *Error) WithError(err error) *Error {
	e.Err = err
	return e
}

func (e *Error) IsRetryable() bool {
	return e.Retryable
}

func (e *Error) GetStatusCode() int {
	return e.StatusCode
}

func (e *Error) MarshalJSON() ([]byte, error) {
	type Alias Error
	return json.Marshal(&struct {
		Error string `json:"error"`
		*Alias
	}{
		Error: e.Message,
		Alias: (*Alias)(e),
	})
}

func New(code, message string) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func Newf(code, format string, args ...interface{}) *Error {
	return &Error{
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func Wrap(err error, code, message string) *Error {
	if err == nil {
		return nil
	}

	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr
	}

	return &Error{
		Code:       code,
		Message:    message,
		Err:        err,
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func Wrapf(err error, code, format string, args ...interface{}) *Error {
	if err == nil {
		return nil
	}

	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr
	}

	return &Error{
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		Err:        err,
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func captureStack(skip int) []StackFrame {
	const maxStackDepth = 32
	pcs := make([]uintptr, maxStackDepth)
	n := runtime.Callers(skip, pcs)
	
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	stack := make([]StackFrame, 0, n)

	for {
		frame, more := frames.Next()
		stack = append(stack, StackFrame{
			File:     frame.File,
			Line:     frame.Line,
			Function: frame.Function,
		})

		if !more {
			break
		}
	}

	return stack
}

func NotFound(resource, id string) *Error {
	return &Error{
		Code:       ErrNotFound,
		Message:    fmt.Sprintf("%s not found", resource),
		Details:    fmt.Sprintf("%s with id '%s' does not exist", resource, id),
		StatusCode: http.StatusNotFound,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func AlreadyExists(resource, id string) *Error {
	return &Error{
		Code:       ErrAlreadyExists,
		Message:    fmt.Sprintf("%s already exists", resource),
		Details:    fmt.Sprintf("%s with id '%s' already exists", resource, id),
		StatusCode: http.StatusConflict,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func InvalidInput(field, reason string) *Error {
	return &Error{
		Code:       ErrInvalidInput,
		Message:    "Invalid input",
		Details:    fmt.Sprintf("Field '%s': %s", field, reason),
		StatusCode: http.StatusBadRequest,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func Unauthorized(message string) *Error {
	return &Error{
		Code:       ErrUnauthorized,
		Message:    "Unauthorized",
		Details:    message,
		StatusCode: http.StatusUnauthorized,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func Forbidden(message string) *Error {
	return &Error{
		Code:       ErrForbidden,
		Message:    "Forbidden",
		Details:    message,
		StatusCode: http.StatusForbidden,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func Internal(message string) *Error {
	return &Error{
		Code:       ErrInternal,
		Message:    "Internal server error",
		Details:    message,
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  true,
	}
}

func Database(operation string, err error) *Error {
	message := "Database error"
	details := fmt.Sprintf("Failed to %s", operation)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &Error{
				Code:       ErrNotFound,
				Message:    "Record not found",
				Details:    details,
				Err:        err,
				StatusCode: http.StatusNotFound,
				Timestamp:  time.Now(),
				Stack:      captureStack(2),
				Retryable:  false,
			}
		}

		details = fmt.Sprintf("%s: %v", details, err)
	}

	return &Error{
		Code:       ErrDatabase,
		Message:    message,
		Details:    details,
		Err:        err,
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  true,
	}
}

func RateLimitExceeded(limit int, window time.Duration) *Error {
	return &Error{
		Code:       ErrRateLimitExceeded,
		Message:    "Rate limit exceeded",
		Details:    fmt.Sprintf("Maximum %d requests per %v", limit, window),
		StatusCode: http.StatusTooManyRequests,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  true,
	}
}

func AccountSuspended(accountID, reason string) *Error {
	return &Error{
		Code:       ErrAccountSuspended,
		Message:    "Account suspended",
		Details:    fmt.Sprintf("Account %s suspended: %s", accountID, reason),
		StatusCode: http.StatusForbidden,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
		Metadata: map[string]interface{}{
			"account_id": accountID,
			"reason":     reason,
		},
	}
}

func DailyLimitExceeded(accountID string, limit int) *Error {
	return &Error{
		Code:       ErrDailyLimitExceeded,
		Message:    "Daily limit exceeded",
		Details:    fmt.Sprintf("Account %s exceeded daily limit of %d", accountID, limit),
		StatusCode: http.StatusTooManyRequests,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  true,
		Metadata: map[string]interface{}{
			"account_id": accountID,
			"limit":      limit,
		},
	}
}

func CampaignError(campaignID, operation string, err error) *Error {
	return &Error{
		Code:       ErrCampaignOperation,
		Message:    fmt.Sprintf("Campaign %s failed", operation),
		Details:    fmt.Sprintf("Campaign %s: %v", campaignID, err),
		Err:        err,
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
		Metadata: map[string]interface{}{
			"campaign_id": campaignID,
			"operation":   operation,
		},
	}
}

func InvalidState(resource, currentState, requiredState string) *Error {
	return &Error{
		Code:    ErrInvalidState,
		Message: "Invalid state transition",
		Details: fmt.Sprintf("%s is in state '%s', but '%s' is required",
			resource, currentState, requiredState),
		StatusCode: http.StatusConflict,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
		Metadata: map[string]interface{}{
			"current_state":  currentState,
			"required_state": requiredState,
		},
	}
}

func TemplateError(templateID, operation string, err error) *Error {
	return &Error{
		Code:       ErrTemplateOperation,
		Message:    fmt.Sprintf("Template %s failed", operation),
		Details:    fmt.Sprintf("Template %s: %v", templateID, err),
		Err:        err,
		StatusCode: http.StatusBadRequest,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
		Metadata: map[string]interface{}{
			"template_id": templateID,
			"operation":   operation,
		},
	}
}

func EmailSendError(recipientEmail, reason string, retryable bool) *Error {
	return &Error{
		Code:       ErrEmailSend,
		Message:    "Failed to send email",
		Details:    fmt.Sprintf("Recipient: %s, Reason: %s", recipientEmail, reason),
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  retryable,
		Metadata: map[string]interface{}{
			"recipient": recipientEmail,
			"reason":    reason,
		},
	}
}

func SMTPError(host string, code int, message string) *Error {
	return &Error{
		Code:       ErrSMTP,
		Message:    "SMTP error",
		Details:    fmt.Sprintf("Host: %s, Code: %d, Message: %s", host, code, message),
		StatusCode: http.StatusBadGateway,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  isRetryableSMTPCode(code),
		Metadata: map[string]interface{}{
			"host":      host,
			"smtp_code": code,
			"message":   message,
		},
	}
}

func ProxyError(proxyURL, operation string, err error) *Error {
	return &Error{
		Code:       ErrProxyConnection,
		Message:    "Proxy connection failed",
		Details:    fmt.Sprintf("Proxy: %s, Operation: %s, Error: %v", proxyURL, operation, err),
		Err:        err,
		StatusCode: http.StatusBadGateway,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  true,
		Metadata: map[string]interface{}{
			"proxy_url": proxyURL,
			"operation": operation,
		},
	}
}

func OAuth2Error(provider, operation string, err error) *Error {
	return &Error{
		Code:       ErrOAuth2,
		Message:    "OAuth2 authentication failed",
		Details:    fmt.Sprintf("Provider: %s, Operation: %s, Error: %v", provider, operation, err),
		Err:        err,
		StatusCode: http.StatusUnauthorized,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
		Metadata: map[string]interface{}{
			"provider":  provider,
			"operation": operation,
		},
	}
}

func ConfigError(field, reason string) *Error {
	return &Error{
		Code:       ErrConfiguration,
		Message:    "Configuration error",
		Details:    fmt.Sprintf("Field '%s': %s", field, reason),
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
		Metadata: map[string]interface{}{
			"field": field,
		},
	}
}

func FileError(filename, operation string, err error) *Error {
	return &Error{
		Code:       ErrFileOperation,
		Message:    fmt.Sprintf("File %s failed", operation),
		Details:    fmt.Sprintf("File: %s, Error: %v", filename, err),
		Err:        err,
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
		Metadata: map[string]interface{}{
			"filename":  filename,
			"operation": operation,
		},
	}
}

func ValidationError(field string, violations []string) *Error {
	return &Error{
		Code:       ErrValidation,
		Message:    "Validation failed",
		Details:    fmt.Sprintf("Field '%s': %s", field, strings.Join(violations, ", ")),
		StatusCode: http.StatusBadRequest,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
		Metadata: map[string]interface{}{
			"field":      field,
			"violations": violations,
		},
	}
}

func TimeoutError(operation string, timeout time.Duration) *Error {
	return &Error{
		Code:       ErrTimeout,
		Message:    "Operation timeout",
		Details:    fmt.Sprintf("%s timed out after %v", operation, timeout),
		StatusCode: http.StatusGatewayTimeout,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  true,
		Metadata: map[string]interface{}{
			"operation": operation,
			"timeout":   timeout.String(),
		},
	}
}

func ResourceExhausted(resource, details string) *Error {
	return &Error{
		Code:       ErrResourceExhausted,
		Message:    fmt.Sprintf("%s exhausted", resource),
		Details:    details,
		StatusCode: http.StatusServiceUnavailable,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  true,
		Metadata: map[string]interface{}{
			"resource": resource,
		},
	}
}

func CacheError(operation string, err error) *Error {
	return &Error{
		Code:       ErrCache,
		Message:    "Cache operation failed",
		Details:    fmt.Sprintf("Operation: %s, Error: %v", operation, err),
		Err:        err,
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  true,
		Metadata: map[string]interface{}{
			"operation": operation,
		},
	}
}

func Is(err error, code string) bool {
	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr.Code == code
	}
	return false
}

func IsRetryable(err error) bool {
	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr.Retryable
	}
	return false
}

func GetStatusCode(err error) int {
	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr.StatusCode
	}
	return http.StatusInternalServerError
}

func GetCode(err error) string {
	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr.Code
	}
	return ErrInternal
}

func GetMetadata(err error) map[string]interface{} {
	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr.Metadata
	}
	return nil
}

func isRetryableSMTPCode(code int) bool {
	retryableCodes := map[int]bool{
		421: true,
		450: true,
		451: true,
		452: true,
	}
	return retryableCodes[code]
}

func FromStdError(err error) *Error {
	if err == nil {
		return nil
	}

	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr
	}

	return &Error{
		Code:       ErrInternal,
		Message:    "Internal error",
		Details:    err.Error(),
		Err:        err,
		StatusCode: http.StatusInternalServerError,
		Timestamp:  time.Now(),
		Stack:      captureStack(2),
		Retryable:  false,
	}
}

func Cause(err error) error {
	type causer interface {
		Cause() error
	}

	for err != nil {
		cause, ok := err.(causer)
		if !ok {
			break
		}
		err = cause.Cause()
	}

	return err
}

func RootCause(err error) error {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}

func Join(errs ...error) error {
	n := 0
	for _, err := range errs {
		if err != nil {
			n++
		}
	}

	if n == 0 {
		return nil
	}

	e := &multiError{
		errs: make([]error, 0, n),
	}

	for _, err := range errs {
		if err != nil {
			e.errs = append(e.errs, err)
		}
	}

	return e
}

type multiError struct {
	errs []error
}

func (e *multiError) Error() string {
	var messages []string
	for _, err := range e.errs {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "; ")
}

func (e *multiError) Unwrap() []error {
	return e.errs
}

func (e *multiError) Errors() []error {
	return e.errs
}

func IsMultiError(err error) bool {
	_, ok := err.(*multiError)
	return ok
}

func GetMultiErrors(err error) []error {
	if me, ok := err.(*multiError); ok {
		return me.Errors()
	}
	return nil
}
// Add these at the end of the file, after the existing error functions

func BadRequest(message string) *Error {
    return &Error{
        Code:       ErrInvalidInput,
        Message:    "Bad Request",
        Details:    message,
        StatusCode: http.StatusBadRequest,
        Timestamp:  time.Now(),
        Stack:      captureStack(2),
        Retryable:  false,
    }
}

func ValidationFailed(message string) *Error {
    return &Error{
        Code:       ErrValidation,
        Message:    "Validation Failed",
        Details:    message,
        StatusCode: http.StatusUnprocessableEntity,
        Timestamp:  time.Now(),
        Stack:      captureStack(2),
        Retryable:  false,
    }
}

type AppError = Error
