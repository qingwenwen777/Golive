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
	JWT       JWTCfg       `mapstructure:"jwt"`
	Upload    UploadCfg    `mapstructure:"upload"`
	Bootstrap BootstrapCfg `mapstructure:"bootstrap"`
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
	Secret     string        `mapstructure:"secret"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

type UploadCfg struct {
	AvatarDir       string `mapstructure:"avatar_dir"`
	AvatarPublicURL string `mapstructure:"avatar_public_url"`
}

type BootstrapCfg struct {
	DemoUser DemoUserCfg `mapstructure:"demo_user"`
	Admin    AdminCfg    `mapstructure:"admin"`
}

type DemoUserCfg struct {
	Enabled     bool   `mapstructure:"enabled"`
	Username    string `mapstructure:"username"`
	Password    string `mapstructure:"password"`
	CoinBalance int64  `mapstructure:"coin_balance"`
}

type AdminCfg struct {
	Enabled     bool   `mapstructure:"enabled"`
	Username    string `mapstructure:"username"`
	Password    string `mapstructure:"password"`
	DisplayName string `mapstructure:"display_name"`
}

// Load reads config from a yaml file. Pass "" for the default search path.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("./configs")
		v.AddConfigPath("./app/user-service/configs")
	}
	v.AutomaticEnv()
	v.SetEnvPrefix("USERSVC")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &c, nil
}
