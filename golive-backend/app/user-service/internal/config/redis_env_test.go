package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// docker-compose.yml passes GOLIVE_REDIS_PASSWORD as USERSVC_REDIS_PASSWORD;
// the deploy config itself holds no password.
func TestLoad_RedisPasswordFromEnv(t *testing.T) {
	t.Setenv("USERSVC_REDIS_PASSWORD", "from-env")
	cfg, err := Load("../../../../deploy/configs/user-service.yaml")
	require.NoError(t, err)
	require.Equal(t, "from-env", cfg.Redis.Password)
}

// Local development runs Redis without a password.
func TestLoad_LocalConfigHasNoRedisPassword(t *testing.T) {
	t.Setenv("USERSVC_REDIS_PASSWORD", "")
	cfg, err := Load("../../configs/config.yaml")
	require.NoError(t, err)
	require.Empty(t, cfg.Redis.Password)
}
