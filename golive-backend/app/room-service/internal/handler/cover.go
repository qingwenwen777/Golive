package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/uploadimage"
)

const maxCoverSize = 5 << 20

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

	uploadimage.LimitRequestBody(c.Writer, c.Request, maxCoverSize)
	file, err := c.FormFile("file")
	if err != nil {
		if uploadimage.IsTooLarge(err) {
			errcode.Respond(c, errcode.New(400, "cover must be 5MB or smaller"))
			return
		}
		errcode.Respond(c, errcode.New(400, "cover file is required"))
		return
	}
	if file.Size <= 0 || file.Size > maxCoverSize {
		errcode.Respond(c, errcode.New(400, "cover must be 5MB or smaller"))
		return
	}

	base := time.Now().UTC().Format("20060102") + "-" + uuid.NewString()
	name, err := uploadimage.SaveOptimized(file, h.dir, base, "", uploadimage.Options{
		MaxWidth:  1280,
		MaxHeight: 720,
		MaxBytes:  maxCoverSize,
		Quality:   93,
	})
	if err != nil {
		if uploadimage.IsTooLarge(err) {
			errcode.Respond(c, errcode.New(400, "cover must be 5MB or smaller"))
			return
		}
		if uploadimage.IsInvalidUpload(err) {
			errcode.Respond(c, errcode.New(400, "cover must be a valid jpg, png, webp, or gif"))
			return
		}
		errcode.Respond(c, fmt.Errorf("save cover: %w", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": h.publicURL + "/" + name})
}
