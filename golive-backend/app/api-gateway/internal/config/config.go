package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Service   ServiceCfg   `mapstructure:"service"`
	Upstreams UpstreamsCfg `mapstructure:"upstreams"`
	Proxy     ProxyCfg     `mapstructure:"proxy"`
	JWT       JWTCfg       `mapstructure:"jwt"`
	CORS      CORSCfg      `mapstructure:"cors"`
	RateLimit RateLimitCfg `mapstructure:"ratelimit"`
}

type ServiceCfg struct {
	Name      string `mapstructure:"name"`
	HTTPAddr  string `mapstructure:"http_addr"`
	PprofAddr string `mapstructure:"pprof_addr"`
	LogLevel  string `mapstructure:"log_level"`
}

type UpstreamsCfg struct {
	UserService string `mapstructure:"user_service"`
	RoomService string `mapstructure:"room_service"`
	GiftService string `mapstructure:"gift_service"`
}

type ProxyCfg struct {
	Timeout             time.Duration `mapstructure:"timeout"`
	MaxIdleConns        int           `mapstructure:"max_idle_conns"`
	MaxIdleConnsPerHost int           `mapstructure:"max_idle_conns_per_host"`
}

type JWTCfg struct {
	Secret string `mapstructure:"secret"`
}

type CORSCfg struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	MaxAge         int      `mapstructure:"max_age"`
}

type RateLimitCfg struct {
	Enabled    bool    `mapstructure:"enabled"`
	RatePerSec float64 `mapstructure:"rate_per_sec"`
	Burst      int     `mapstructure:"burst"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("./configs")
		v.AddConfigPath("./app/api-gateway/configs")
	}
	v.AutomaticEnv()
	v.SetEnvPrefix("GW")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &c, nil
}
