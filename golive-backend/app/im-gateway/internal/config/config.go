package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

type Config struct {
	Service ServiceCfg `mapstructure:"service"`
	WS      WSCfg      `mapstructure:"ws"`
	Redis   RedisCfg   `mapstructure:"redis"`
	JWT     JWTCfg     `mapstructure:"jwt"`
	Kafka   KafkaCfg   `mapstructure:"kafka"`
	Room    RoomCfg    `mapstructure:"room"`
	Filter  FilterCfg  `mapstructure:"filter"`
	// ChatRateLimit is the per-user (not per-connection) chat limit, shared
	// through Redis by all of a user's connections.
	ChatRateLimit ChatRateLimitCfg `mapstructure:"chat_ratelimit"`
}

type ServiceCfg struct {
	Name      string `mapstructure:"name"`
	HTTPAddr  string `mapstructure:"http_addr"`
	PprofAddr string `mapstructure:"pprof_addr"`
	LogLevel  string `mapstructure:"log_level"`
}

type WSCfg struct {
	ReadLimitBytes  int64         `mapstructure:"read_limit_bytes"`
	ReadIdleTimeout time.Duration `mapstructure:"read_idle_timeout"`
	WriteDeadline   time.Duration `mapstructure:"write_deadline"`
	SendBuffer      int           `mapstructure:"send_buffer"`
	PongWait        time.Duration `mapstructure:"pong_wait"`
	MaxMessageRate  float64       `mapstructure:"max_message_rate"`
	AllowedOrigins  []string      `mapstructure:"allowed_origins"`
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
	ChatTopic string   `mapstructure:"chat_topic"`
}

type RoomCfg struct {
	ViewerPushInterval time.Duration `mapstructure:"viewer_push_interval"`
	WelcomeText        string        `mapstructure:"welcome_text"`
}

// FilterCfg points at the sensitive-word list (one word per line) whose
// matches are masked in live chat.
type FilterCfg struct {
	SensitivePath string `mapstructure:"sensitive_path"`
	Mask          string `mapstructure:"mask"`
}

// ChatRateLimitCfg allows PerUserPerSec chat messages per BucketSeconds
// window. BucketSeconds is an integer number of seconds (not a duration).
type ChatRateLimitCfg struct {
	PerUserPerSec int `mapstructure:"per_user_per_sec"`
	BucketSeconds int `mapstructure:"bucket_seconds"`
}

func (c ChatRateLimitCfg) Window() time.Duration {
	return time.Duration(c.BucketSeconds) * time.Second
}

func (c *Config) validate() error {
	if c.ChatRateLimit.PerUserPerSec <= 0 {
		return fmt.Errorf("chat_ratelimit.per_user_per_sec must be > 0, got %d", c.ChatRateLimit.PerUserPerSec)
	}
	if c.ChatRateLimit.BucketSeconds <= 0 {
		return fmt.Errorf("chat_ratelimit.bucket_seconds must be a positive number of seconds, got %d", c.ChatRateLimit.BucketSeconds)
	}
	if strings.TrimSpace(c.Filter.SensitivePath) == "" {
		return fmt.Errorf("filter.sensitive_path is required")
	}
	return nil
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("./configs")
		v.AddConfigPath("./app/im-gateway/configs")
	}
	v.AutomaticEnv()
	v.SetEnvPrefix("IMGW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = v.BindEnv("redis.password")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &c, nil
}
