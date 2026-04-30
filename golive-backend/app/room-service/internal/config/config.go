package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Service ServiceCfg `mapstructure:"service"`
	MySQL   MySQLCfg   `mapstructure:"mysql"`
	Redis   RedisCfg   `mapstructure:"redis"`
	JWT     JWTCfg     `mapstructure:"jwt"`
	Live    LiveCfg    `mapstructure:"live"`
	Upload  UploadCfg  `mapstructure:"upload"`
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
	Secret string `mapstructure:"secret"`
}

type LiveCfg struct {
	StreamKeySecret string        `mapstructure:"stream_key_secret"`
	StreamKeyTTL    time.Duration `mapstructure:"stream_key_ttl"`
	// FlvBase is the public HTTP-FLV URL prefix (e.g.
	// "http://localhost:8082/live"). Stream IDs/keys are appended with ".flv".
	FlvBase string `mapstructure:"flv_base"`
}

type UploadCfg struct {
	CoverDir       string `mapstructure:"cover_dir"`
	CoverPublicURL string `mapstructure:"cover_public_url"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("./configs")
		v.AddConfigPath("./app/room-service/configs")
	}
	v.AutomaticEnv()
	v.SetEnvPrefix("ROOMSVC")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &c, nil
}
