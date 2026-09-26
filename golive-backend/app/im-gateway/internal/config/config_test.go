package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoad_ShippedConfigsAreValid(t *testing.T) {
	for _, path := range []string{"../../configs/config.yaml", "../../../../deploy/configs/im-gateway.yaml"} {
		cfg, err := Load(path)
		require.NoError(t, err, path)
		require.Equal(t, time.Second, cfg.ChatRateLimit.Window(), path)
		require.NotEmpty(t, cfg.Filter.SensitivePath, path)
	}
}

func TestLoad_RejectsInvalidChatRateLimit(t *testing.T) {
	for name, body := range map[string]string{
		"duration string": "filter:\n  sensitive_path: x\nchat_ratelimit:\n  per_user_per_sec: 3\n  bucket_seconds: 1s\n",
		"zero window":     "filter:\n  sensitive_path: x\nchat_ratelimit:\n  per_user_per_sec: 3\n  bucket_seconds: 0\n",
		"zero limit":      "filter:\n  sensitive_path: x\nchat_ratelimit:\n  per_user_per_sec: 0\n  bucket_seconds: 1\n",
		"no word list":    "chat_ratelimit:\n  per_user_per_sec: 3\n  bucket_seconds: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
			_, err := Load(path)
			require.Error(t, err)
		})
	}
}
