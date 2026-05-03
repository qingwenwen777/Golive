package handler

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/uploadimage"
)

type CoverUploadHandler struct {
	dir       string
	publicURL string
}

func NewCoverUploadHandler(dir, publicURL string) *CoverUploadHandler {
	if dir == "" {
		dir = "./uploads/covers"
	}
	if publicURL == "" {
		publicURL = "/uploads/covers"
	}
	return &CoverUploadHandler{dir: dir, publicURL: strings.TrimRight(publicURL, "/")}
}

func (h *CoverUploadHandler) Upload(c *gin.Context) {
	if UserIDFromCtx(c) == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		errcode.Respond(c, errcode.New(400, "cover file is required"))
		return
	}
	if file.Size <= 0 || file.Size > 5<<20 {
		errcode.Respond(c, errcode.New(400, "cover must be 5MB or smaller"))
		return
	}

	contentType := file.Header.Get("Content-Type")
	ext, ok := allowedCoverExt(contentType)
	if !ok {
		ext = strings.ToLower(filepath.Ext(file.Filename))
		if !allowedExt(ext) {
			errcode.Respond(c, errcode.New(400, "cover must be jpg, png, webp, or gif"))
			return
		}
	}

	base := time.Now().UTC().Format("20060102") + "-" + uuid.NewString()
	name, err := uploadimage.SaveOptimized(file, h.dir, base, ext, uploadimage.Options{
		MaxWidth:  1280,
		MaxHeight: 720,
		Quality:   93,
	})
	if err != nil {
		errcode.Respond(c, fmt.Errorf("save cover: %w", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": h.publicURL + "/" + name})
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

func allowedExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}
