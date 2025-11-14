package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Azure     AzureConfig     `yaml:"azure"`
	Database  DatabaseConfig  `yaml:"database"`
	Server    ServerConfig    `yaml:"server"`
	Audio     AudioConfig     `yaml:"audio"`
}

// AzureConfig holds Azure Cognitive Services credentials
type AzureConfig struct {
	SubscriptionKey string            `yaml:"subscription_key"`
	Region          string            `yaml:"region"`
	MaxQPS          float64           `yaml:"max_qps"` // Maximum queries per second
	Voices          map[string]string `yaml:"voices"`  // Custom voice mappings (language_code -> voice_name)
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	Path        string `yaml:"path"`
	Compression bool   `yaml:"compression"` // Enable zstd compression for cached audio
}

// ServerConfig holds gRPC server settings
type ServerConfig struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

// AudioConfig holds audio playback settings
type AudioConfig struct {
	SampleRate  int `yaml:"sample_rate"`
	BufferSize  int `yaml:"buffer_size"`
}

// Load reads and parses the configuration file
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate required fields
	if config.Azure.SubscriptionKey == "" {
		return nil, fmt.Errorf("azure.subscription_key is required")
	}
	if config.Azure.Region == "" {
		return nil, fmt.Errorf("azure.region is required")
	}

	// Set default for MaxQPS if not specified
	if config.Azure.MaxQPS <= 0 {
		config.Azure.MaxQPS = 10.0 // Default: 10 requests per second
	}

	// Set defaults
	if config.Database.Path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		config.Database.Path = filepath.Join(homeDir, ".local", "share", "tts-daemon", "cache.db")
	}

	if config.Server.Address == "" {
		config.Server.Address = "localhost"
	}
	if config.Server.Port == 0 {
		config.Server.Port = 50051
	}

	if config.Audio.SampleRate == 0 {
		config.Audio.SampleRate = 44100
	}
	if config.Audio.BufferSize == 0 {
		config.Audio.BufferSize = 4096
	}

	return &config, nil
}

// GetDefaultConfigPath returns the default configuration file path
func GetDefaultConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "tts-daemon", "config.yaml"), nil
}
