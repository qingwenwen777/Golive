package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
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

	contentType := file.Header.Get("Content-Type")
	ext, ok := allowedCoverExt(contentType)
	if !ok {
		ext = strings.ToLower(filepath.Ext(file.Filename))
		if !allowedCoverFileExt(ext) {
			errcode.Respond(c, errcode.New(http.StatusBadRequest, "cover must be jpg, png, webp, or gif"))
			return
		}
	}

	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		errcode.Respond(c, fmt.Errorf("create upload dir: %w", err))
		return
	}
	name := time.Now().UTC().Format("20060102") + "-" + uuid.NewString() + ext
	url := h.publicURL + "/" + name
	if err := c.SaveUploadedFile(file, filepath.Join(h.dir, name)); err != nil {
		errcode.Respond(c, fmt.Errorf("save cover: %w", err))
		return
	}

	u, err := h.users.UpdateCover(c.Request.Context(), uid, url)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url, "user": u.Public()})
}

func allowedCoverExt(contentType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	case "image/gif":
		return ".gif", true
	default:
		return "", false
	}
}

func allowedCoverFileExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}
