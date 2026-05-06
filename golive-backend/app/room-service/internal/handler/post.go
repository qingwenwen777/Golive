package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/uploadimage"
)

type PostHandler struct {
	svc        *service.PostService
	permission service.LivePermissionChecker
	imageDir   string
	publicURL  string
}

func NewPostHandler(svc *service.PostService, permission service.LivePermissionChecker, imageDir, publicURL string) *PostHandler {
	if imageDir == "" {
		imageDir = "./uploads/posts"
	}
	if publicURL == "" {
		publicURL = "/uploads/posts"
	}
	return &PostHandler{
		svc:        svc,
		permission: permission,
		imageDir:   imageDir,
		publicURL:  strings.TrimRight(publicURL, "/"),
	}
}

func (h *PostHandler) ListMine(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	page, size := postPageQuery(c, 1, 10)
	resp, err := h.svc.ListMine(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *PostHandler) ListChannel(c *gin.Context) {
	page, size := postPageQuery(c, 1, 6)
	resp, err := h.svc.ListChannel(c.Request.Context(), UserIDFromCtx(c), c.Param("channel"), page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *PostHandler) ListSubscriptionLatest(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	_, size := postPageQuery(c, 1, 8)
	resp, err := h.svc.ListSubscriptionLatest(c.Request.Context(), uid, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *PostHandler) Create(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	var req service.CreatePostReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	post, err := h.svc.CreatePost(c.Request.Context(), uid, req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) UpdateVisibility(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	var req service.UpdatePostVisibilityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	post, err := h.svc.UpdatePostVisibility(c.Request.Context(), uid, c.Param("postID"), req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, post)
}

func (h *PostHandler) Delete(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	if err := h.svc.DeletePost(c.Request.Context(), uid, c.Param("postID")); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *PostHandler) UploadImage(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	if h.permission != nil {
		approved, err := h.permission.HasApprovedLivePermission(c.Request.Context(), uid)
		if err != nil {
			errcode.Respond(c, err)
			return
		}
		if !approved {
			errcode.Respond(c, errcode.New(http.StatusForbidden, "creator permission is not approved").WithReason("creator_permission_required"))
			return
		}
	}

	file, err := c.FormFile("file")
	if err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "post image is required"))
		return
	}
	if file.Size <= 0 || file.Size > 5<<20 {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "post image must be 5MB or smaller"))
		return
	}

	base := time.Now().UTC().Format("20060102") + "-" + uuid.NewString()
	name, err := uploadimage.SaveOptimized(file, h.imageDir, base, "", uploadimage.Options{
		MaxWidth:  1600,
		MaxHeight: 1600,
		Quality:   92,
	})
	if err != nil {
		if uploadimage.IsInvalidUpload(err) {
			errcode.Respond(c, errcode.New(http.StatusBadRequest, "post image must be a valid jpg, png, webp, or gif"))
			return
		}
		errcode.Respond(c, fmt.Errorf("save post image: %w", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": h.publicURL + "/" + name})
}

func (h *PostHandler) ListComments(c *gin.Context) {
	resp, err := h.svc.ListComments(c.Request.Context(), UserIDFromCtx(c), c.Param("postID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *PostHandler) CreateComment(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	var req service.CreateCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	comment, err := h.svc.CreateComment(c.Request.Context(), uid, c.Param("postID"), req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, comment)
}

func (h *PostHandler) DeleteComment(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	if err := h.svc.DeleteComment(c.Request.Context(), uid, c.Param("postID"), c.Param("commentID")); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *PostHandler) Like(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	state, err := h.svc.LikePost(c.Request.Context(), uid, c.Param("postID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *PostHandler) Unlike(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	state, err := h.svc.UnlikePost(c.Request.Context(), uid, c.Param("postID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *PostHandler) LikeComment(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	state, err := h.svc.LikeComment(c.Request.Context(), uid, c.Param("postID"), c.Param("commentID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *PostHandler) UnlikeComment(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	state, err := h.svc.UnlikeComment(c.Request.Context(), uid, c.Param("postID"), c.Param("commentID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func postPageQuery(c *gin.Context, defaultPage, defaultSize int) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(defaultPage)))
	size, _ := strconv.Atoi(c.DefaultQuery("size", strconv.Itoa(defaultSize)))
	return page, size
}
