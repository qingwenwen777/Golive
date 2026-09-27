package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Both configs spell out the limits on retrying failed replay uploads.
func TestLoad_ReplayUploadRetrySettings(t *testing.T) {
	for _, path := range []string{"../../../../deploy/configs/room-service.yaml", "../../configs/config.yaml"} {
		cfg, err := Load(path)
		require.NoError(t, err, path)
		require.Equal(t, 7, cfg.Replay.UploadAttempts, path)
		require.Equal(t, 15*time.Minute, cfg.Replay.UploadRetryDelay, path)
		require.Equal(t, 7*24*time.Hour, cfg.Replay.FailedRecordingRetention, path)
	}
}
