package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type UserHandler struct {
	users *repo.UserRepo
}

func NewUserHandler(users *repo.UserRepo) *UserHandler {
	return &UserHandler{users: users}
}

// Me returns the authenticated user's profile. AuthRequired middleware has
// already validated the JWT and stashed the userId in the context.
func (h *UserHandler) Me(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	u, err := h.users.FindByID(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	c.JSON(http.StatusOK, u.Public())
}

// PublicProfile returns a public user profile by id, with username as a
// compatibility fallback for channel links that may carry a handle.
func (h *UserHandler) PublicProfile(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "missing user id"))
		return
	}

	u, err := h.users.FindByID(c.Request.Context(), id)
	if errors.Is(err, repo.ErrUserNotFound) {
		u, err = h.users.FindByUsername(c.Request.Context(), id)
	}
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, u.Public())
}

type topupReq struct {
	Amount int64 `json:"amount"`
}

// TopupCoins increments the authenticated user's coin balance by the given
// amount and returns the updated public user. This is a stub for development;
// real billing integration is out of scope.
func (h *UserHandler) TopupCoins(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	var req topupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid amount"})
		return
	}
	if req.Amount <= 0 || req.Amount > 1_000_000 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid amount"})
		return
	}
	u, err := h.users.IncrementCoins(c.Request.Context(), uid, req.Amount)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	c.JSON(http.StatusOK, u.Public())
}
