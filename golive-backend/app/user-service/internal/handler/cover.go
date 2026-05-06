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

const maxCoverSize = 5 << 20

type CoverUploadHandler struct {
	users     *repo.UserRepo
	dir       string
	publicURL string
}

func NewCoverUploadHandler(users *repo.UserRepo, dir, publicURL string) *CoverUploadHandler {
	if dir == "" {
		dir = "./uploads/covers"
	}
	if publicURL == "" {
		publicURL = "/api/uploads/covers"
	}
	return &CoverUploadHandler{
		users:     users,
		dir:       dir,
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

func (h *CoverUploadHandler) Upload(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "cover file is required"))
		return
	}
	if file.Size <= 0 || file.Size > maxCoverSize {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "cover must be 5MB or smaller"))
		return
	}

	base := time.Now().UTC().Format("20060102") + "-" + uuid.NewString()
	name, err := uploadimage.SaveOptimized(file, h.dir, base, "", uploadimage.Options{
		MaxWidth:  1600,
		MaxHeight: 900,
		Quality:   93,
	})
	if err != nil {
		if uploadimage.IsInvalidUpload(err) {
			errcode.Respond(c, errcode.New(http.StatusBadRequest, "cover must be a valid jpg, png, webp, or gif"))
			return
		}
		errcode.Respond(c, fmt.Errorf("save cover: %w", err))
		return
	}
	url := h.publicURL + "/" + name

	u, err := h.users.UpdateCover(c.Request.Context(), uid, url)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url, "user": u.Public()})
}
