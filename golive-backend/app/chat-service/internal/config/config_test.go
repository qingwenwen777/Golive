package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// bucket_seconds: 1 used to decode into a time.Duration of 1ns, so the
// per-user limiter never limited anything.
func TestLoad_BucketSecondsIsSeconds(t *testing.T) {
	cfg, err := Load(writeConfig(t, "ratelimit:\n  per_user_per_sec: 3\n  bucket_seconds: 1\n"))
	require.NoError(t, err)
	require.Equal(t, time.Second, cfg.RateLimit.Window())
	require.Equal(t, 3, cfg.RateLimit.PerUserPerSec)
}

func TestLoad_RejectsInvalidRateLimit(t *testing.T) {
	for name, body := range map[string]string{
		"zero window":     "ratelimit:\n  per_user_per_sec: 3\n  bucket_seconds: 0\n",
		"missing window":  "ratelimit:\n  per_user_per_sec: 3\n",
		"duration string": "ratelimit:\n  per_user_per_sec: 3\n  bucket_seconds: 1s\n",
		"zero limit":      "ratelimit:\n  per_user_per_sec: 0\n  bucket_seconds: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeConfig(t, body))
			require.Error(t, err)
		})
	}
}

func TestLoad_InternalTokenFromEnv(t *testing.T) {
	t.Setenv("CHATSVC_INTERNAL_TOKEN", "from-env")
	cfg, err := Load("../../../../deploy/configs/chat-service.yaml")
	require.NoError(t, err)
	require.Equal(t, "from-env", cfg.Internal.Token)
}

// Nothing produces to Kafka by default, so the consumer must stay off unless
// explicitly enabled.
func TestLoad_KafkaDisabledByDefault(t *testing.T) {
	for _, path := range []string{"../../configs/config.yaml", "../../../../deploy/configs/chat-service.yaml"} {
		cfg, err := Load(path)
		require.NoError(t, err, path)
		require.False(t, cfg.Kafka.Enabled, path)
	}

	cfg, err := Load(writeConfig(t, "ratelimit:\n  per_user_per_sec: 3\n  bucket_seconds: 1\n"))
	require.NoError(t, err)
	require.False(t, cfg.Kafka.Enabled, "missing key")
}

func TestLoad_KafkaEnabledFromEnv(t *testing.T) {
	t.Setenv("CHATSVC_KAFKA_ENABLED", "true")
	cfg, err := Load("../../../../deploy/configs/chat-service.yaml")
	require.NoError(t, err)
	require.True(t, cfg.Kafka.Enabled)

	// Also when the file has no kafka section at all.
	cfg, err = Load(writeConfig(t, "ratelimit:\n  per_user_per_sec: 3\n  bucket_seconds: 1\n"))
	require.NoError(t, err)
	require.True(t, cfg.Kafka.Enabled)
}

func TestLoad_ShippedConfigsAreValid(t *testing.T) {
	for _, path := range []string{"../../configs/config.yaml", "../../../../deploy/configs/chat-service.yaml"} {
		cfg, err := Load(path)
		require.NoError(t, err, path)
		require.Equal(t, time.Second, cfg.RateLimit.Window(), path)
	}
}
