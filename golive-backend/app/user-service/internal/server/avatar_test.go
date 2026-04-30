package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUploadAvatarReturnsRelativeURLAndPersistsUser(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	uploadDir := t.TempDir()
	router = NewRouter(Deps{
		Auth:            auth,
		Users:           users,
		AvatarDir:       uploadDir,
		AvatarPublicURL: "/api/uploads/avatars",
	})

	login, err := auth.Register(context.Background(), "demo", "demo", "Demo")
	require.NoError(t, err)

	body, contentType := multipartBody(t, "face.png", "image/png", []byte("png"))
	req := httptest.NewRequest(http.MethodPost, "/users/me/avatar", body)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		URL  string `json:"url"`
		User struct {
			Avatar string `json:"avatar"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.True(t, strings.HasPrefix(got.URL, "/api/uploads/avatars/"))
	require.Equal(t, got.URL, got.User.Avatar)

	persisted, err := users.FindByID(context.Background(), login.User.ID)
	require.NoError(t, err)
	require.Equal(t, got.URL, persisted.Avatar)
	require.FileExists(t, filepath.Join(uploadDir, filepath.Base(got.URL)))
}

func TestUploadAvatarRejectsInvalidType(t *testing.T) {
	router, _, auth := newCoinsTestRouter(t)
	login, err := auth.Register(context.Background(), "demo", "demo", "Demo")
	require.NoError(t, err)

	body, contentType := multipartBody(t, "notes.txt", "text/plain", []byte("nope"))
	req := httptest.NewRequest(http.MethodPost, "/users/me/avatar", body)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func multipartBody(t *testing.T, filename, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	})
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return body, w.FormDataContentType()
}
