package config

import (
	"fmt"
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

type RateLimitCfg struct {
	PerUserPerSec  int           `mapstructure:"per_user_per_sec"`
	BucketSeconds  time.Duration `mapstructure:"bucket_seconds"`
}

type RoomCfg struct {
	HistoryDefaultLimit int `mapstructure:"history_default_limit"`
	HistoryMaxLimit     int `mapstructure:"history_max_limit"`
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
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &c, nil
}
