package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type SearchHandler struct {
	svc *service.SearchService
}

func NewSearchHandler(svc *service.SearchService) *SearchHandler {
	return &SearchHandler{svc: svc}
}

func (h *SearchHandler) Search(c *gin.Context) {
	size, _ := strconv.Atoi(c.DefaultQuery("size", "8"))
	resp, err := h.svc.Search(c.Request.Context(), UserIDFromCtx(c), c.Query("q"), size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SearchHandler) Suggest(c *gin.Context) {
	size, _ := strconv.Atoi(c.DefaultQuery("size", "12"))
	resp, err := h.svc.Suggest(c.Request.Context(), UserIDFromCtx(c), c.Query("q"), size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
