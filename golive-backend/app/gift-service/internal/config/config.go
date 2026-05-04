package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"

	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

type Config struct {
	Service     ServiceCfg     `mapstructure:"service"`
	MySQL       MySQLCfg       `mapstructure:"mysql"`
	Redis       RedisCfg       `mapstructure:"redis"`
	JWT         JWTCfg         `mapstructure:"jwt"`
	Kafka       KafkaCfg       `mapstructure:"kafka"`
	Outbox      OutboxCfg      `mapstructure:"outbox"`
	Idempotency IdempotencyCfg `mapstructure:"idempotency"`
}

type ServiceCfg struct {
	Name      string `mapstructure:"name"`
	HTTPAddr  string `mapstructure:"http_addr"`
	PprofAddr string `mapstructure:"pprof_addr"`
	LogLevel  string `mapstructure:"log_level"`
}
type MySQLCfg struct {
	DSN     string `mapstructure:"dsn"`
	MaxOpen int    `mapstructure:"max_open"`
	MaxIdle int    `mapstructure:"max_idle"`
}
type RedisCfg struct {
	Addr     string `mapstructure:"addr"`
	DB       int    `mapstructure:"db"`
	Password string `mapstructure:"password"`
}
type JWTCfg struct {
	Secret    string      `mapstructure:"secret"`
	ActiveKID string      `mapstructure:"active_kid"`
	Secrets   []JWTKeyCfg `mapstructure:"secrets"`
}

type JWTKeyCfg = jwtauth.KeyConfig

func (c JWTCfg) KeySet() (*jwtauth.KeySet, error) {
	return jwtauth.NewKeySet(c.Secret, c.ActiveKID, c.Secrets)
}

type KafkaCfg struct {
	Enabled   bool     `mapstructure:"enabled"`
	Brokers   []string `mapstructure:"brokers"`
	GiftTopic string   `mapstructure:"gift_topic"`
}
type OutboxCfg struct {
	PollInterval time.Duration `mapstructure:"poll_interval"`
	BatchSize    int           `mapstructure:"batch_size"`
	MaxRetries   int           `mapstructure:"max_retries"`
	BaseBackoff  time.Duration `mapstructure:"base_backoff"`
}
type IdempotencyCfg struct {
	TTL time.Duration `mapstructure:"ttl"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("./configs")
		v.AddConfigPath("./app/gift-service/configs")
	}
	v.AutomaticEnv()
	v.SetEnvPrefix("GIFTSVC")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &c, nil
}
