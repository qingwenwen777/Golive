package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type SocialHandler struct {
	svc *service.SocialService
}

func NewSocialHandler(svc *service.SocialService) *SocialHandler {
	return &SocialHandler{svc: svc}
}

func (h *SocialHandler) require(c *gin.Context) (string, bool) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return "", false
	}
	return uid, true
}

// follow ---------------------------------------------------------------

func (h *SocialHandler) GetFollow(c *gin.Context) {
	uid := UserIDFromCtx(c)
	state, err := h.svc.GetFollow(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *SocialHandler) ListSubscriptions(c *gin.Context) {
	uid, ok := h.require(c)
	if !ok {
		return
	}
	resp, err := h.svc.ListSubscriptions(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SocialHandler) RecommendedCreators(c *gin.Context) {
	size, _ := strconv.Atoi(c.DefaultQuery("size", "8"))
	resp, err := h.svc.RecommendedCreators(c.Request.Context(), UserIDFromCtx(c), size, c.Query("category"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SocialHandler) Follow(c *gin.Context) {
	uid, ok := h.require(c)
	if !ok {
		return
	}
	state, err := h.svc.Follow(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *SocialHandler) Unfollow(c *gin.Context) {
	uid, ok := h.require(c)
	if !ok {
		return
	}
	state, err := h.svc.Unfollow(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

// like / dislike -------------------------------------------------------

func (h *SocialHandler) GetLike(c *gin.Context) {
	uid, ok := h.require(c)
	if !ok {
		return
	}
	state, err := h.svc.GetLike(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *SocialHandler) Like(c *gin.Context) {
	uid, ok := h.require(c)
	if !ok {
		return
	}
	state, err := h.svc.Like(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *SocialHandler) Unlike(c *gin.Context) {
	uid, ok := h.require(c)
	if !ok {
		return
	}
	state, err := h.svc.Unlike(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *SocialHandler) Dislike(c *gin.Context) {
	uid, ok := h.require(c)
	if !ok {
		return
	}
	state, err := h.svc.Dislike(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *SocialHandler) Undislike(c *gin.Context) {
	uid, ok := h.require(c)
	if !ok {
		return
	}
	state, err := h.svc.Undislike(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}
