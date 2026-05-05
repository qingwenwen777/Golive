package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
)

func TestCreatorApplicationApprovalGrantsLivePermissionOnly(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))

	creator, err := auth.Register(ctx, "creator", "secret123", "Creator")
	require.NoError(t, err)
	require.False(t, creator.User.Verified)

	submitReq := httptest.NewRequest(http.MethodPost, "/creator/applications", bytes.NewBufferString(`{
		"reason":"I want live permission."
	}`))
	submitReq.Header.Set("Authorization", "Bearer "+creator.Token)
	submitReq.Header.Set("Content-Type", "application/json")
	submitRec := httptest.NewRecorder()
	router.ServeHTTP(submitRec, submitReq)
	require.Equal(t, http.StatusCreated, submitRec.Code)

	var submitted struct {
		Application model.CreatorApplication `json:"application"`
		User        model.PublicUser         `json:"user"`
	}
	require.NoError(t, json.Unmarshal(submitRec.Body.Bytes(), &submitted))
	require.Equal(t, model.LivePermissionPending, submitted.User.LivePermissionStatus)
	require.False(t, submitted.User.Verified)

	approveReq := httptest.NewRequest(http.MethodPost, "/admin/creator-applications/"+submitted.Application.ID+"/approve", nil)
	approveReq.Header.Set("Authorization", "Bearer "+admin.Token)
	approveRec := httptest.NewRecorder()
	router.ServeHTTP(approveRec, approveReq)
	require.Equal(t, http.StatusOK, approveRec.Code)

	var approved struct {
		User model.PublicUser `json:"user"`
	}
	require.NoError(t, json.Unmarshal(approveRec.Body.Bytes(), &approved))
	require.Equal(t, model.LivePermissionApproved, approved.User.LivePermissionStatus)
	require.Equal(t, model.PlatformVerificationNone, approved.User.PlatformVerificationStatus)
	require.False(t, approved.User.Verified)

	persisted, err := users.FindByID(ctx, creator.User.ID)
	require.NoError(t, err)
	require.False(t, persisted.Verified)
}

func TestPlatformApplicationRequiresLivePermissionThenApprovesVerification(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	admin, err := auth.Register(ctx, "admin2", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin2"))

	creator, err := auth.Register(ctx, "creator2", "secret123", "Creator 2")
	require.NoError(t, err)

	blockedReq := httptest.NewRequest(http.MethodPost, "/creator/platform-applications", bytes.NewBufferString(`{
		"reason":"I want platform certification."
	}`))
	blockedReq.Header.Set("Authorization", "Bearer "+creator.Token)
	blockedReq.Header.Set("Content-Type", "application/json")
	blockedRec := httptest.NewRecorder()
	router.ServeHTTP(blockedRec, blockedReq)
	require.Equal(t, http.StatusConflict, blockedRec.Code)

	_, err = users.SetLivePermissionStatus(ctx, creator.User.ID, model.LivePermissionApproved)
	require.NoError(t, err)

	submitReq := httptest.NewRequest(http.MethodPost, "/creator/platform-applications", bytes.NewBufferString(`{
		"reason":"I want to sign with the platform."
	}`))
	submitReq.Header.Set("Authorization", "Bearer "+creator.Token)
	submitReq.Header.Set("Content-Type", "application/json")
	submitRec := httptest.NewRecorder()
	router.ServeHTTP(submitRec, submitReq)
	require.Equal(t, http.StatusCreated, submitRec.Code)

	var submitted struct {
		Application model.PlatformApplication `json:"application"`
		User        model.PublicUser          `json:"user"`
	}
	require.NoError(t, json.Unmarshal(submitRec.Body.Bytes(), &submitted))
	require.Equal(t, model.PlatformVerificationPending, submitted.User.PlatformVerificationStatus)
	require.False(t, submitted.User.Verified)

	approveReq := httptest.NewRequest(http.MethodPost, "/admin/platform-applications/"+submitted.Application.ID+"/approve", nil)
	approveReq.Header.Set("Authorization", "Bearer "+admin.Token)
	approveRec := httptest.NewRecorder()
	router.ServeHTTP(approveRec, approveReq)
	require.Equal(t, http.StatusOK, approveRec.Code)

	var approved struct {
		User model.PublicUser `json:"user"`
	}
	require.NoError(t, json.Unmarshal(approveRec.Body.Bytes(), &approved))
	require.Equal(t, model.LivePermissionApproved, approved.User.LivePermissionStatus)
	require.Equal(t, model.PlatformVerificationApproved, approved.User.PlatformVerificationStatus)
	require.True(t, approved.User.Verified)

	persisted, err := users.FindByID(ctx, creator.User.ID)
	require.NoError(t, err)
	require.True(t, persisted.Verified)
}

func TestPlatformApplicationRejectRequiresReasonAndClearsVerification(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	admin, err := auth.Register(ctx, "admin3", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin3"))

	creator, err := auth.Register(ctx, "creator3", "secret123", "Creator 3")
	require.NoError(t, err)
	_, err = users.SetLivePermissionStatus(ctx, creator.User.ID, model.LivePermissionApproved)
	require.NoError(t, err)

	app, _, _, err := users.SubmitPlatformApplication(ctx, creator.User.ID, "Please certify my channel.")
	require.NoError(t, err)

	rejectWithoutReasonReq := httptest.NewRequest(http.MethodPost, "/admin/platform-applications/"+app.ID+"/reject", bytes.NewBufferString(`{}`))
	rejectWithoutReasonReq.Header.Set("Authorization", "Bearer "+admin.Token)
	rejectWithoutReasonReq.Header.Set("Content-Type", "application/json")
	rejectWithoutReasonRec := httptest.NewRecorder()
	router.ServeHTTP(rejectWithoutReasonRec, rejectWithoutReasonReq)
	require.Equal(t, http.StatusBadRequest, rejectWithoutReasonRec.Code)

	rejectReq := httptest.NewRequest(http.MethodPost, "/admin/platform-applications/"+app.ID+"/reject", bytes.NewBufferString(`{
		"reason":"Please complete a stable streaming schedule first."
	}`))
	rejectReq.Header.Set("Authorization", "Bearer "+admin.Token)
	rejectReq.Header.Set("Content-Type", "application/json")
	rejectRec := httptest.NewRecorder()
	router.ServeHTTP(rejectRec, rejectReq)
	require.Equal(t, http.StatusOK, rejectRec.Code)

	var rejected struct {
		User model.PublicUser `json:"user"`
	}
	require.NoError(t, json.Unmarshal(rejectRec.Body.Bytes(), &rejected))
	require.Equal(t, model.LivePermissionApproved, rejected.User.LivePermissionStatus)
	require.Equal(t, model.PlatformVerificationRejected, rejected.User.PlatformVerificationStatus)
	require.False(t, rejected.User.Verified)
	require.NotEmpty(t, rejected.User.PlatformVerificationRejectReason)

	_, err = auth.Login(ctx, "creator3", "secret123")
	require.NoError(t, err)
}

func TestReconcilePlatformVerificationMatchesCertificationStatus(t *testing.T) {
	_, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	admin, err := auth.Register(ctx, "admin4", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin4"))
	hash, err := service.HashPassword("secret123")
	require.NoError(t, err)

	legacy := &model.User{
		ID:                   "legacy-google-id",
		Username:             "legacygoogle",
		Email:                "legacy-google@example.com",
		DisplayName:          "Legacy",
		PasswordHash:         hash,
		Avatar:               "https://example.com/avatar.png",
		CoinBalance:          1200,
		Verified:             true,
		Role:                 model.RoleUser,
		LivePermissionStatus: model.LivePermissionApproved,
	}
	require.NoError(t, users.Create(ctx, legacy))

	certified := &model.User{
		ID:                         "certified-id",
		Username:                   "certified",
		Email:                      "certified@example.com",
		DisplayName:                "Certified",
		PasswordHash:               hash,
		Avatar:                     "https://example.com/certified.png",
		CoinBalance:                1200,
		Verified:                   false,
		Role:                       model.RoleUser,
		LivePermissionStatus:       model.LivePermissionApproved,
		PlatformVerificationStatus: model.PlatformVerificationApproved,
	}
	require.NoError(t, users.Create(ctx, certified))

	require.NoError(t, users.ReconcilePlatformVerification(ctx))

	got, err := users.FindByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.False(t, got.Verified)
	adminUser, err := users.FindByID(ctx, admin.User.ID)
	require.NoError(t, err)
	require.False(t, adminUser.Verified)
	certifiedUser, err := users.FindByID(ctx, certified.ID)
	require.NoError(t, err)
	require.True(t, certifiedUser.Verified)
}
