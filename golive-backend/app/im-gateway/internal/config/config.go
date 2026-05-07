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
	return &c, nil
}
