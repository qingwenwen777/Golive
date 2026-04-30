// SRS hooks: see https://ossrs.net/lts/zh-cn/docs/v5/doc/http-callback
//
// SRS treats HTTP 2xx + body `{"code":0}` as accept; any non-zero code or
// non-2xx status as reject. We always reply 200 and put the verdict in the
// body so the response shape is uniform.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/logger"
)

type SRSHandler struct {
	svc *service.LiveService
}

func NewSRSHandler(svc *service.LiveService) *SRSHandler {
	return &SRSHandler{svc: svc}
}

func (h *SRSHandler) OnPublish(c *gin.Context) {
	var req service.SRSPublishReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.L().Warn("srs on_publish bad body", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "bad body"})
		return
	}
	if err := h.svc.OnPublish(c.Request.Context(), req); err != nil {
		logger.L().Warn("srs on_publish reject",
			zap.String("stream", req.Stream), zap.Error(err))
		c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "reject"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0})
}

func (h *SRSHandler) OnUnpublish(c *gin.Context) {
	var req service.SRSPublishReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0}) // best-effort
		return
	}
	_ = h.svc.OnUnpublish(c.Request.Context(), req)
	c.JSON(http.StatusOK, gin.H{"code": 0})
}
