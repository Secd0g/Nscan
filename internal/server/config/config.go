package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig `mapstructure:"server"`
	MongoDB MongoConfig  `mapstructure:"mongodb"`
	Redis   RedisConfig  `mapstructure:"redis"`
	Log     LogConfig    `mapstructure:"log"`
	Queue   QueueConfig  `mapstructure:"queue"`
}

type QueueConfig struct {
	// Mode controls dispatch strategy:
	//   "redis"  → Phase-3 subtask split+distribute (requires Redis)
	//   "legacy" → single-node PickNode+gRPC (default, backward-compatible)
	Mode string `mapstructure:"mode"`
}

type ServerConfig struct {
	HTTPAddr     string    `mapstructure:"http_addr"`
	GRPCAddr     string    `mapstructure:"grpc_addr"`
	TLS          TLSConfig `mapstructure:"tls"`
	AuthToken    string    `mapstructure:"auth_token"`
	AdminUser    string    `mapstructure:"admin_user"`
	AdminPass    string    `mapstructure:"admin_pass"`
	JWTSecret    string    `mapstructure:"jwt_secret"`
	ScannerImage string    `mapstructure:"scanner_image"`
}

type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

type MongoConfig struct {
	URI      string `mapstructure:"uri"`
	Database string `mapstructure:"database"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var cfg Config
	return &cfg, v.Unmarshal(&cfg)
}
