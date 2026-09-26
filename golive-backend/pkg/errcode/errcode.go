// Package errcode unifies HTTP error responses into { message, reason? }.
//
// Usage:
//
//	if balance < price {
//	    errcode.Respond(c, errcode.ErrInsufficientCoin)
//	    return
//	}
//	errcode.Respond(c, errcode.New(http.StatusBadRequest, "missing requestId"))
package errcode

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/pkg/logger"
)

// AppError is the canonical error surfaced to the frontend.
type AppError struct {
	HTTPStatus int    `json:"-"`
	Message    string `json:"message"`
	Reason     string `json:"reason,omitempty"`
}

func (e *AppError) Error() string { return e.Message }

// New builds a fresh AppError.
func New(status int, message string) *AppError {
	return &AppError{HTTPStatus: status, Message: message}
}

// WithReason sets a machine-readable reason (frontend branches on this).
func (e *AppError) WithReason(reason string) *AppError {
	clone := *e
	clone.Reason = reason
	return &clone
}

// Pre-defined errors -------------------------------------------------------

var (
	ErrUnauthorized     = New(http.StatusUnauthorized, "unauthorized")
	ErrBadRequest       = New(http.StatusBadRequest, "bad request")
	ErrNotFound         = New(http.StatusNotFound, "not found")
	ErrInternal         = New(http.StatusInternalServerError, "internal error")
	ErrMissingRequestID = New(http.StatusBadRequest, "missing requestId")

	// ErrInsufficientCoin is HTTP 402 with reason=insufficient_coin.
	// Gift / SuperChat handlers must ALSO return a failed order object alongside.
	ErrInsufficientCoin = &AppError{
		HTTPStatus: http.StatusPaymentRequired,
		Message:    "insufficient coin balance",
		Reason:     "insufficient_coin",
	}
)

// Respond writes err as the unified JSON body. Non-AppError falls back to a
// generic 500; the underlying error (SQL text, internal addresses, ...) is
// logged, never sent to the client.
func Respond(c *gin.Context, err error) {
	var ae *AppError
	if errors.As(err, &ae) {
		c.AbortWithStatusJSON(ae.HTTPStatus, ae)
		return
	}
	logger.L().Error("internal error",
		zap.Error(err),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
		zap.String("request_id", c.GetHeader("X-Request-Id")),
	)
	c.AbortWithStatusJSON(http.StatusInternalServerError, ErrInternal)
}

// RespondWith writes status + AppError plus an extra payload (e.g. a failed order
// object). The outer response merges AppError fields with payload under a single
// JSON object. Use for /gifts/send and /super-chats insufficient_coin case.
func RespondWith(c *gin.Context, ae *AppError, payload gin.H) {
	body := gin.H{"message": ae.Message}
	if ae.Reason != "" {
		body["reason"] = ae.Reason
	}
	for k, v := range payload {
		body[k] = v
	}
	c.AbortWithStatusJSON(ae.HTTPStatus, body)
}
