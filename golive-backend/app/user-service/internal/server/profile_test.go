package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
)

func TestPublicProfileReturnsPublicUserByID(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo Creator")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/users/profile/"+login.User.ID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got model.PublicUser
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, login.User.ID, got.ID)
	require.Equal(t, "Demo Creator", got.DisplayName)
}
