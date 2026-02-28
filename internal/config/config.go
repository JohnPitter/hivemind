package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	ConfigPath string `mapstructure:"-"`

	// Server settings
	API APIConfig `mapstructure:"api"`

	// Room defaults
	Room RoomConfig `mapstructure:"room"`

	// Worker settings
	Worker WorkerConfig `mapstructure:"worker"`

	// Logging
	Log LogConfig `mapstructure:"log"`
}

// APIConfig holds HTTP API server settings.
type APIConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	RateLimit    int    `mapstructure:"rate_limit"`
	MaxBodyBytes int64  `mapstructure:"max_body_bytes"`
}

// RoomConfig holds default room settings.
type RoomConfig struct {
	MaxPeers     int  `mapstructure:"max_peers"`
	AutoApprove  bool `mapstructure:"auto_approve"`
	InviteLength int  `mapstructure:"invite_length"`
}

// WorkerConfig holds Python worker settings.
type WorkerConfig struct {
	GRPCPort       int `mapstructure:"grpc_port"`
	HealthInterval int `mapstructure:"health_interval_s"`
	MaxRestarts    int `mapstructure:"max_restarts"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Load reads configuration from file and environment variables.
func Load(cfgFile string) (*Config, error) {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}

		configDir := filepath.Join(home, ".hivemind")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return nil, fmt.Errorf("create config dir: %w", err)
		}

		viper.AddConfigPath(configDir)
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	setDefaults()

	viper.SetEnvPrefix("HIVEMIND")
	viper.AutomaticEnv()

	// Config file is optional — defaults are fine
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.ConfigPath = viper.ConfigFileUsed()

	return &cfg, nil
}

func setDefaults() {
	// API
	viper.SetDefault("api.host", "127.0.0.1")
	viper.SetDefault("api.port", 8080)
	viper.SetDefault("api.rate_limit", 60)
	viper.SetDefault("api.max_body_bytes", 10*1024*1024) // 10MB

	// Room
	viper.SetDefault("room.max_peers", 10)
	viper.SetDefault("room.auto_approve", true)
	viper.SetDefault("room.invite_length", 12)

	// Worker
	viper.SetDefault("worker.grpc_port", 50051)
	viper.SetDefault("worker.health_interval_s", 5)
	viper.SetDefault("worker.max_restarts", 3)

	// Log
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")
}
