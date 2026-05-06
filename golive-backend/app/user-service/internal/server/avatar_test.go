package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
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

	body, contentType := multipartBody(t, "face.png", "image/png", pngUpload(t))
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

	body, contentType := multipartBody(t, "notes.png", "image/png", []byte("nope"))
	req := httptest.NewRequest(http.MethodPost, "/users/me/avatar", body)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUploadCoverReturnsRelativeURLAndPersistsUser(t *testing.T) {
	router, users, auth := newCoinsTestRouter(t)
	coverDir := t.TempDir()
	router = NewRouter(Deps{
		Auth:           auth,
		Users:          users,
		CoverDir:       coverDir,
		CoverPublicURL: "/api/uploads/covers",
	})

	login, err := auth.Register(context.Background(), "demo", "demo", "Demo")
	require.NoError(t, err)

	body, contentType := multipartBody(t, "banner.png", "image/png", pngUpload(t))
	req := httptest.NewRequest(http.MethodPost, "/users/me/cover", body)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		URL  string `json:"url"`
		User struct {
			Cover string `json:"cover"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.True(t, strings.HasPrefix(got.URL, "/api/uploads/covers/"))
	require.Equal(t, got.URL, got.User.Cover)

	persisted, err := users.FindByID(context.Background(), login.User.ID)
	require.NoError(t, err)
	require.Equal(t, got.URL, persisted.Cover)
	require.FileExists(t, filepath.Join(coverDir, filepath.Base(got.URL)))
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

func pngUpload(t *testing.T) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			img.Set(x, y, color.NRGBA{
				R: uint8(30 + x*10),
				G: uint8(60 + y*12),
				B: 180,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
