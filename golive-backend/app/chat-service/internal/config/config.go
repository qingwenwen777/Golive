package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Service   ServiceCfg   `mapstructure:"service"`
	MySQL     MySQLCfg     `mapstructure:"mysql"`
	Redis     RedisCfg     `mapstructure:"redis"`
	Kafka     KafkaCfg     `mapstructure:"kafka"`
	Filter    FilterCfg    `mapstructure:"filter"`
	RateLimit RateLimitCfg `mapstructure:"ratelimit"`
	Room      RoomCfg      `mapstructure:"room"`
	Internal  InternalCfg  `mapstructure:"internal"`
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
	Shards  int    `mapstructure:"shards"`
}

type RedisCfg struct {
	Addr     string `mapstructure:"addr"`
	DB       int    `mapstructure:"db"`
	Password string `mapstructure:"password"`
}

type KafkaCfg struct {
	Brokers []string `mapstructure:"brokers"`
	Topic   string   `mapstructure:"topic"`
	Group   string   `mapstructure:"group"`
	Workers int      `mapstructure:"workers"`
}

type FilterCfg struct {
	SensitivePath string `mapstructure:"sensitive_path"`
	Mask          string `mapstructure:"mask"`
}

// RateLimitCfg is the per-user chat limit: PerUserPerSec messages per
// BucketSeconds-long window. BucketSeconds is a plain integer number of
// seconds — decoding it straight into a time.Duration turned the YAML value
// `1` into 1ns, which gave every message its own window and never limited.
type RateLimitCfg struct {
	PerUserPerSec int `mapstructure:"per_user_per_sec"`
	BucketSeconds int `mapstructure:"bucket_seconds"`
}

// Window returns the limiter window as a duration.
func (c RateLimitCfg) Window() time.Duration {
	return time.Duration(c.BucketSeconds) * time.Second
}

func (c RateLimitCfg) validate() error {
	if c.PerUserPerSec <= 0 {
		return fmt.Errorf("ratelimit.per_user_per_sec must be > 0, got %d", c.PerUserPerSec)
	}
	if c.BucketSeconds <= 0 {
		return fmt.Errorf("ratelimit.bucket_seconds must be a positive number of seconds, got %d", c.BucketSeconds)
	}
	return nil
}

type RoomCfg struct {
	HistoryDefaultLimit int `mapstructure:"history_default_limit"`
	HistoryMaxLimit     int `mapstructure:"history_max_limit"`
}

// InternalCfg holds the shared secret other services send on /internal
// calls (CHATSVC_INTERNAL_TOKEN). Empty rejects every internal call.
type InternalCfg struct {
	Token string `mapstructure:"token"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("./configs")
		v.AddConfigPath("./app/chat-service/configs")
	}
	v.AutomaticEnv()
	v.SetEnvPrefix("CHATSVC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = v.BindEnv("mysql.dsn")
	_ = v.BindEnv("redis.password")
	_ = v.BindEnv("internal.token")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := c.RateLimit.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &c, nil
}
