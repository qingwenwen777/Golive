package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type SuperChatHandler struct {
	svc  *service.SuperChatService
	idem *service.IdemCache
}

func NewSuperChatHandler(s *service.SuperChatService, idem *service.IdemCache) *SuperChatHandler {
	return &SuperChatHandler{svc: s, idem: idem}
}

type sendScBody struct {
	RoomID      string `json:"roomId"`
	Amount      int64  `json:"amount"`
	Text        string `json:"text"`
	DisplayName string `json:"displayName"`
	RequestID   string `json:"requestId"`
}

func (h *SuperChatHandler) Send(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return
	}
	var body sendScBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(400, "Bad request"))
		return
	}
	if strings.TrimSpace(body.RoomID) == "" || body.Amount <= 0 {
		errcode.Respond(c, errcode.New(400, "Bad request"))
		return
	}

	requestID := strings.TrimSpace(c.GetHeader(headerRequestID))
	if requestID == "" {
		requestID = strings.TrimSpace(body.RequestID)
	}
	if requestID == "" {
		errcode.Respond(c, errcode.New(400, "Missing requestId"))
		return
	}

	if cached, err := h.idem.Lookup(c.Request.Context(), uid, requestID); err == nil && cached != nil {
		c.Writer.Header().Set(headerReplayed, "true")
		c.Data(cached.Status, "application/json; charset=utf-8", cached.Body)
		return
	}

	order, replayed, sErr := h.svc.Send(c.Request.Context(), service.SendSuperChatReq{
		UserID:    uid,
		Username:  body.DisplayName,
		RoomID:    body.RoomID,
		Amount:    body.Amount,
		Text:      body.Text,
		RequestID: requestID,
	})
	if errors.Is(sErr, service.ErrInvalidAmount) {
		errcode.Respond(c, errcode.New(400, "Amount below minimum tier"))
		return
	}

	switch {
	case sErr == nil:
		respondSC(c, http.StatusOK, order, replayed, h.idem, uid, requestID)
	case errors.Is(sErr, service.ErrInsufficientCoin):
		respondSCFailure(c, order, h.idem, uid, requestID)
	default:
		errcode.Respond(c, sErr)
	}
}

func respondSC(c *gin.Context, status int, order *model.SuperChatOrder, replayed bool,
	idem *service.IdemCache, userID, requestID string) {

	body, _ := json.Marshal(order)
	if replayed {
		c.Writer.Header().Set(headerReplayed, "true")
	}
	c.Data(status, "application/json; charset=utf-8", body)
	_ = idem.Save(c.Request.Context(), userID, requestID, status, body)
}

func respondSCFailure(c *gin.Context, order *model.SuperChatOrder,
	idem *service.IdemCache, userID, requestID string) {

	merged := map[string]any{
		"orderId":    order.OrderID,
		"requestId":  order.RequestID,
		"amount":     order.Amount,
		"tier":       order.Tier,
		"text":       order.Text,
		"status":     order.Status,
		"failReason": order.FailReason,
		"createdAt":  order.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		"reason":     "insufficient_coin",
		"message":    "Insufficient coins",
	}
	body, _ := json.Marshal(merged)
	c.Data(http.StatusPaymentRequired, "application/json; charset=utf-8", body)
	_ = idem.Save(c.Request.Context(), userID, requestID, http.StatusPaymentRequired, body)
}
