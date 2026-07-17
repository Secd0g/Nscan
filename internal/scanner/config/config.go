package config

import "github.com/spf13/viper"

type Config struct {
	Scanner ScannerConfig `mapstructure:"scanner"`
	Log     LogConfig     `mapstructure:"log"`
}

type ScannerConfig struct {
	Name         string      `mapstructure:"name"`
	ServerAddr   string      `mapstructure:"server_addr"`
	ServerHTTP   string      `mapstructure:"server_http"` // HTTP API base URL, e.g. http://localhost:8080
	Token        string      `mapstructure:"token"`
	MaxTasks     int32       `mapstructure:"max_tasks"`
	Capabilities []string    `mapstructure:"capabilities"`
	TLS          TLSConfig   `mapstructure:"tls"`
	DataDir      string      `mapstructure:"data_dir"` // persistent data directory, default "./data/scanner"
	// Phase 3: subtask queue worker
	Queue        QueueConfig `mapstructure:"queue"`
}

type QueueConfig struct {
	RedisAddr  string `mapstructure:"redis_addr"`  // e.g. "localhost:6379"
	RedisPass  string `mapstructure:"redis_pass"`
	NumWorkers int    `mapstructure:"num_workers"` // goroutines per capability; 0 = disabled
}

type TLSConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	CAFile             string `mapstructure:"ca_file"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
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
