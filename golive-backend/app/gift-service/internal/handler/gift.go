package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const headerRequestID = "X-Request-Id"
const headerReplayed = "Idempotent-Replayed"

type GiftHandler struct {
	svc  *service.GiftService
	idem *service.IdemCache
}

func NewGiftHandler(s *service.GiftService, idem *service.IdemCache) *GiftHandler {
	return &GiftHandler{svc: s, idem: idem}
}

func (h *GiftHandler) List(c *gin.Context) {
	gifts, err := h.svc.List(c.Request.Context())
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gifts)
}

func (h *GiftHandler) FanBadges(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return
	}
	badges, err := h.svc.ListFanBadges(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, badges)
}

func (h *GiftHandler) FanClubMembers(c *gin.Context) {
	creatorID := strings.TrimSpace(c.Param("creatorID"))
	if creatorID == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	limit := 5
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	resp, err := h.svc.ListFanClubMembers(c.Request.Context(), creatorID, limit)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

type sendGiftBody struct {
	RoomID    string `json:"roomId"`
	GiftID    string `json:"giftId"`
	Count     int    `json:"count"`
	RequestID string `json:"requestId"`
}

type joinFanClubBody struct {
	CreatorID string `json:"creatorId"`
	RequestID string `json:"requestId"`
}

// Send mirrors POST /api/gifts/send. Response shapes (must match
// src/mocks/handlers/gift.ts):
//
//	200  body=GiftOrder                          (success or replay)
//	402  body={...GiftOrder, reason, message}    (insufficient_coin)
//	400  body={message}                          (bad request / missing requestId)
//	401  body={message:"Unauthorized"}
//	404  body={message:"Gift not found"}
//
// The Idempotent-Replayed: true header is set whenever we serve a previously
// computed result — both success replays and failure replays.
func (h *GiftHandler) Send(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return
	}

	var body sendGiftBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(400, "Bad request"))
		return
	}
	if strings.TrimSpace(body.RoomID) == "" || strings.TrimSpace(body.GiftID) == "" || body.Count <= 0 {
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

	// 1) Hot path: Redis idempotency cache.
	if cached, err := h.idem.Lookup(c.Request.Context(), uid, requestID); err == nil && cached != nil {
		c.Writer.Header().Set(headerReplayed, "true")
		c.Data(cached.Status, "application/json; charset=utf-8", cached.Body)
		return
	}

	// 2) Hand to the service. It owns the DB-level idempotency / balance check.
	order, replayed, sErr := h.svc.Send(c.Request.Context(), service.SendGiftReq{
		UserID:    uid,
		Username:  uid,
		RoomID:    body.RoomID,
		GiftID:    body.GiftID,
		Count:     body.Count,
		RequestID: requestID,
	})
	if sErr != nil && errors.Is(sErr, service.ErrGiftNotFound) {
		errcode.Respond(c, errcode.New(404, "Gift not found"))
		return
	}
	var locked *service.GiftLevelLockedError
	if sErr != nil && errors.As(sErr, &locked) {
		c.JSON(http.StatusForbidden, gin.H{
			"message":       "Gift unlock level not reached",
			"reason":        "gift_level_locked",
			"requiredLevel": locked.RequiredLevel,
			"userLevel":     locked.UserLevel,
		})
		return
	}

	// 3) Render + cache.
	switch {
	case sErr == nil:
		respondGift(c, http.StatusOK, order, replayed, h.idem, uid, requestID)
	case errors.Is(sErr, service.ErrInsufficientCoin):
		respondGiftFailure(c, order, h.idem, uid, requestID)
	default:
		errcode.Respond(c, sErr)
	}
}

func (h *GiftHandler) JoinFanClub(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return
	}

	var body joinFanClubBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(400, "Bad request"))
		return
	}
	body.CreatorID = strings.TrimSpace(body.CreatorID)
	if body.CreatorID == "" {
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

	order, replayed, sErr := h.svc.JoinFanClub(c.Request.Context(), service.JoinFanClubReq{
		UserID:    uid,
		CreatorID: body.CreatorID,
		RequestID: requestID,
	})
	if sErr != nil && errors.Is(sErr, service.ErrGiftNotFound) {
		errcode.Respond(c, errcode.New(404, "Creator or fan light not found"))
		return
	}
	if sErr != nil && errors.Is(sErr, service.ErrSelfFanClubJoin) {
		errcode.Respond(c, errcode.New(409, "Cannot join your own fan club"))
		return
	}
	var locked *service.GiftLevelLockedError
	if sErr != nil && errors.As(sErr, &locked) {
		c.JSON(http.StatusForbidden, gin.H{
			"message":       "Gift unlock level not reached",
			"reason":        "gift_level_locked",
			"requiredLevel": locked.RequiredLevel,
			"userLevel":     locked.UserLevel,
		})
		return
	}
	switch {
	case sErr == nil:
		respondGift(c, http.StatusOK, order, replayed, h.idem, uid, requestID)
	case errors.Is(sErr, service.ErrInsufficientCoin):
		respondGiftFailure(c, order, h.idem, uid, requestID)
	default:
		errcode.Respond(c, sErr)
	}
}

// respondGift writes a success / replay response and caches it.
func respondGift(c *gin.Context, status int, order *model.GiftOrder, replayed bool,
	idem *service.IdemCache, userID, requestID string) {

	body, _ := json.Marshal(order)
	if replayed {
		c.Writer.Header().Set(headerReplayed, "true")
	}
	c.Data(status, "application/json; charset=utf-8", body)
	_ = idem.Save(c.Request.Context(), userID, requestID, status, body)
}

// respondGiftFailure renders the 402 shape: order fields flattened with
// reason+message keys alongside. Mirrors src/mocks/handlers/gift.ts exactly.
func respondGiftFailure(c *gin.Context, order *model.GiftOrder,
	idem *service.IdemCache, userID, requestID string) {

	merged := map[string]any{
		"orderId":    order.OrderID,
		"requestId":  order.RequestID,
		"giftId":     order.GiftID,
		"count":      order.Count,
		"totalCoin":  order.TotalCoin,
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
