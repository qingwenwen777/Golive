package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWaitForStableFileWaitsThroughGrowth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.flv")
	require.NoError(t, os.WriteFile(path, []byte("a"), 0o644))

	growthDone := make(chan struct{})
	go func() {
		defer close(growthDone)
		for i := 0; i < 3; i++ {
			time.Sleep(8 * time.Millisecond)
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				return
			}
			_, _ = file.WriteString("a")
			_ = file.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	info, err := waitForStableFile(ctx, path, 5*time.Millisecond, 4)
	require.NoError(t, err)
	<-growthDone
	require.Equal(t, int64(4), info.Size())
}

func TestUploadVideoUsesStableContentLengthWhenFileGrows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.flv")
	require.NoError(t, os.WriteFile(path, []byte("abc"), 0o644))

	type uploadResult struct {
		bodyLength    int
		contentLength int64
		err           error
	}
	resultCh := make(chan uploadResult, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			resultCh <- uploadResult{err: fmt.Errorf("unexpected method %s", r.Method)}
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			resultCh <- uploadResult{err: err}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = file.WriteString("def")
		closeErr := file.Close()
		if err != nil {
			resultCh <- uploadResult{err: err}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if closeErr != nil {
			resultCh <- uploadResult{err: closeErr}
			http.Error(w, closeErr.Error(), http.StatusInternalServerError)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			resultCh <- uploadResult{err: err}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resultCh <- uploadResult{bodyLength: len(body), contentLength: r.ContentLength}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewBunnyClient(server.URL, "test-key", time.Second)
	require.NoError(t, client.UploadVideo(context.Background(), "lib", "video", path))

	result := <-resultCh
	require.NoError(t, result.err)
	require.Equal(t, int64(3), result.contentLength)
	require.Equal(t, 3, result.bodyLength)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, int64(6), info.Size())
}
