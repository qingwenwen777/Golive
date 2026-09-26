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
	MySQL   MySQLCfg   `mapstructure:"mysql"`
	Redis   RedisCfg   `mapstructure:"redis"`
	JWT     JWTCfg     `mapstructure:"jwt"`
	Live    LiveCfg    `mapstructure:"live"`
	Replay  ReplayCfg  `mapstructure:"replay"`
	Upload  UploadCfg  `mapstructure:"upload"`
	Users   UsersCfg   `mapstructure:"users"`
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

type LiveCfg struct {
	StreamKeySecret string        `mapstructure:"stream_key_secret"`
	StreamKeyTTL    time.Duration `mapstructure:"stream_key_ttl"`
	// FlvBase is the public HTTP-FLV URL prefix (e.g.
	// "http://localhost:8082/live"). Stream IDs/keys are appended with ".flv".
	FlvBase string `mapstructure:"flv_base"`
	// SRSAPIBase is the SRS HTTP API (e.g. "http://srs:1985"), used to kick
	// the publisher when a room is stopped. Empty disables kicking.
	SRSAPIBase string `mapstructure:"srs_api_base"`
}

type ReplayCfg struct {
	RecordDir       string        `mapstructure:"record_dir"`
	BunnyLibraryID  string        `mapstructure:"bunny_library_id"`
	BunnyAPIKey     string        `mapstructure:"bunny_api_key"`
	BunnyAPIBase    string        `mapstructure:"bunny_api_base"`
	BunnyPlayerBase string        `mapstructure:"bunny_player_base"`
	UploadTimeout   time.Duration `mapstructure:"upload_timeout"`
}

type UploadCfg struct {
	CoverDir       string `mapstructure:"cover_dir"`
	CoverPublicURL string `mapstructure:"cover_public_url"`
	PostImageDir   string `mapstructure:"post_image_dir"`
	PostPublicURL  string `mapstructure:"post_public_url"`
}

type UsersCfg struct {
	ServiceURL string `mapstructure:"service_url"`
	GRPCAddr   string `mapstructure:"grpc_addr"`
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
	v.SetEnvPrefix("ROOMSVC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	_ = v.BindEnv("mysql.dsn")
	_ = v.BindEnv("redis.password")
	_ = v.BindEnv("live.stream_key_secret")
	_ = v.BindEnv("replay.bunny_library_id")
	_ = v.BindEnv("replay.bunny_api_key")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &c, nil
}
