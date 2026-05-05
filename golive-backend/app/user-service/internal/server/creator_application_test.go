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

func TestCreatorApplicationApprovalSetsPlatformVerification(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))

	creator, err := auth.Register(ctx, "creator", "secret123", "Creator")
	require.NoError(t, err)
	require.False(t, creator.User.Verified)

	submitReq := httptest.NewRequest(http.MethodPost, "/creator/applications", bytes.NewBufferString(`{
		"reason":"I want to join the platform creator program."
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
	require.True(t, approved.User.Verified)

	persisted, err := users.FindByID(ctx, creator.User.ID)
	require.NoError(t, err)
	require.True(t, persisted.Verified)
}

func TestCreatorApplicationRejectRequiresReasonAndClearsVerification(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)

	admin, err := auth.Register(context.Background(), "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(context.Background(), "admin"))

	creator, err := auth.Register(context.Background(), "creator2", "secret123", "Creator 2")
	require.NoError(t, err)

	app, _, _, err := users.SubmitCreatorApplication(context.Background(), creator.User.ID, "Please review.")
	require.NoError(t, err)

	rejectWithoutReasonReq := httptest.NewRequest(http.MethodPost, "/admin/creator-applications/"+app.ID+"/reject", bytes.NewBufferString(`{}`))
	rejectWithoutReasonReq.Header.Set("Authorization", "Bearer "+admin.Token)
	rejectWithoutReasonReq.Header.Set("Content-Type", "application/json")
	rejectWithoutReasonRec := httptest.NewRecorder()
	router.ServeHTTP(rejectWithoutReasonRec, rejectWithoutReasonReq)
	require.Equal(t, http.StatusBadRequest, rejectWithoutReasonRec.Code)

	rejectReq := httptest.NewRequest(http.MethodPost, "/admin/creator-applications/"+app.ID+"/reject", bytes.NewBufferString(`{
		"reason":"Please complete the channel profile first."
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
	require.Equal(t, model.LivePermissionRejected, rejected.User.LivePermissionStatus)
	require.False(t, rejected.User.Verified)
	require.NotEmpty(t, rejected.User.LivePermissionRejectReason)

	_, err = auth.Login(context.Background(), "creator2", "secret123")
	require.NoError(t, err)
}

func TestReconcilePlatformVerificationMatchesApplicationStatus(t *testing.T) {
	_, users, auth := newCoinsTestRouter(t)
	ctx := context.Background()

	admin, err := auth.Register(ctx, "admin", "secret123", "Admin")
	require.NoError(t, err)
	require.NoError(t, users.EnsureAdmin(ctx, "admin"))
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
		LivePermissionStatus: model.LivePermissionNone,
	}
	require.NoError(t, users.Create(ctx, legacy))

	require.NoError(t, users.ReconcilePlatformVerification(ctx))

	got, err := users.FindByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.False(t, got.Verified)
	adminUser, err := users.FindByID(ctx, admin.User.ID)
	require.NoError(t, err)
	require.True(t, adminUser.Verified)
}
