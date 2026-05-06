package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/qingwenwen777/golive/pkg/jwtauth"
)

type Config struct {
	Service   ServiceCfg   `mapstructure:"service"`
	MySQL     MySQLCfg     `mapstructure:"mysql"`
	Redis     RedisCfg     `mapstructure:"redis"`
	JWT       JWTCfg       `mapstructure:"jwt"`
	Google    GoogleCfg    `mapstructure:"google"`
	Email     EmailCfg     `mapstructure:"email"`
	Upload    UploadCfg    `mapstructure:"upload"`
	Stripe    StripeCfg    `mapstructure:"stripe"`
	Bootstrap BootstrapCfg `mapstructure:"bootstrap"`
}

type ServiceCfg struct {
	Name      string `mapstructure:"name"`
	HTTPAddr  string `mapstructure:"http_addr"`
	GRPCAddr  string `mapstructure:"grpc_addr"`
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
	ActiveKID  string        `mapstructure:"active_kid"`
	Secrets    []JWTKeyCfg   `mapstructure:"secrets"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

type JWTKeyCfg = jwtauth.KeyConfig

func (c JWTCfg) KeySet() (*jwtauth.KeySet, error) {
	return jwtauth.NewKeySet(c.Secret, c.ActiveKID, c.Secrets)
}

type GoogleCfg struct {
	ClientID string `mapstructure:"client_id"`
}

type EmailCfg struct {
	Enabled        bool          `mapstructure:"enabled"`
	SMTPHost       string        `mapstructure:"smtp_host"`
	SMTPPort       int           `mapstructure:"smtp_port"`
	SMTPUsername   string        `mapstructure:"smtp_username"`
	SMTPPassword   string        `mapstructure:"smtp_password"`
	SenderEmail    string        `mapstructure:"sender_email"`
	SenderName     string        `mapstructure:"sender_name"`
	CodeTTL        time.Duration `mapstructure:"code_ttl"`
	ResendInterval time.Duration `mapstructure:"resend_interval"`
}

type UploadCfg struct {
	AvatarDir       string `mapstructure:"avatar_dir"`
	AvatarPublicURL string `mapstructure:"avatar_public_url"`
	CoverDir        string `mapstructure:"cover_dir"`
	CoverPublicURL  string `mapstructure:"cover_public_url"`
}

type StripeCfg struct {
	PublishableKey       string `mapstructure:"publishable_key"`
	SecretKey            string `mapstructure:"secret_key"`
	Currency             string `mapstructure:"currency"`
	CoinsPerCurrencyUnit int64  `mapstructure:"coins_per_currency_unit"`
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
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = v.BindEnv("google.client_id")
	_ = v.BindEnv("email.enabled")
	_ = v.BindEnv("email.smtp_host")
	_ = v.BindEnv("email.smtp_port")
	_ = v.BindEnv("email.smtp_username")
	_ = v.BindEnv("email.smtp_password")
	_ = v.BindEnv("email.sender_email")
	_ = v.BindEnv("email.sender_name")
	_ = v.BindEnv("email.code_ttl")
	_ = v.BindEnv("email.resend_interval")
	_ = v.BindEnv("stripe.publishable_key")
	_ = v.BindEnv("stripe.secret_key")
	_ = v.BindEnv("stripe.currency")
	_ = v.BindEnv("stripe.coins_per_currency_unit")
	_ = v.BindEnv("stripe.connect_country")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &c, nil
}
