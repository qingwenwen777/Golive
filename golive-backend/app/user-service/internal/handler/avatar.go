package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/uploadimage"
)

const maxAvatarSize = 5 << 20

type AvatarUploadHandler struct {
	users     *repo.UserRepo
	dir       string
	publicURL string
}

func NewAvatarUploadHandler(users *repo.UserRepo, dir, publicURL string) *AvatarUploadHandler {
	if dir == "" {
		dir = "./uploads/avatars"
	}
	if publicURL == "" {
		publicURL = "/api/uploads/avatars"
	}
	return &AvatarUploadHandler{
		users:     users,
		dir:       dir,
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

func (h *AvatarUploadHandler) Upload(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}

	uploadimage.LimitRequestBody(c.Writer, c.Request, maxAvatarSize)
	file, err := c.FormFile("file")
	if err != nil {
		if uploadimage.IsTooLarge(err) {
			errcode.Respond(c, errcode.New(http.StatusBadRequest, "avatar must be 5MB or smaller"))
			return
		}
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "avatar file is required"))
		return
	}
	if file.Size <= 0 || file.Size > maxAvatarSize {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "avatar must be 5MB or smaller"))
		return
	}

	base := time.Now().UTC().Format("20060102") + "-" + uuid.NewString()
	name, err := uploadimage.SaveOptimized(file, h.dir, base, "", uploadimage.Options{
		MaxWidth:  512,
		MaxHeight: 512,
		MaxBytes:  maxAvatarSize,
		Quality:   94,
	})
	if err != nil {
		if uploadimage.IsTooLarge(err) {
			errcode.Respond(c, errcode.New(http.StatusBadRequest, "avatar must be 5MB or smaller"))
			return
		}
		if uploadimage.IsInvalidUpload(err) {
			errcode.Respond(c, errcode.New(http.StatusBadRequest, "avatar must be a valid jpg, png, webp, or gif"))
			return
		}
		errcode.Respond(c, fmt.Errorf("save avatar: %w", err))
		return
	}
	url := h.publicURL + "/" + name

	u, err := h.users.UpdateAvatar(c.Request.Context(), uid, url)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url, "user": u.Public()})
}
