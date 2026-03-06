package config

import (
	"encoding/hex"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DB       DBConfig       `yaml:"db"`
	Facebook FacebookConfig `yaml:"facebook"`
	Sync     SyncConfig     `yaml:"sync"`
	Security SecurityConfig `yaml:"security"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

type DBConfig struct {
	DSN string `yaml:"dsn"`
}

type FacebookConfig struct {
	APIVersion string `yaml:"api_version"`
}

type SyncConfig struct {
	Schedule    string `yaml:"schedule"`
	BackfillDays int   `yaml:"backfill_days"`
}

type SecurityConfig struct {
	EncryptionKey string `yaml:"encryption_key"`
}

// Load reads config from the given YAML file path.
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// EncryptionKeyBytes decodes the hex encryption key to raw bytes.
func (c *Config) EncryptionKeyBytes() ([]byte, error) {
	key, err := hex.DecodeString(c.Security.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	return key, nil
}

func (c *Config) validate() error {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.DB.DSN == "" {
		c.DB.DSN = "./data.db"
	}
	if c.Facebook.APIVersion == "" {
		c.Facebook.APIVersion = "v20.0"
	}
	if c.Sync.Schedule == "" {
		c.Sync.Schedule = "0 1 * * *"
	}
	if c.Sync.BackfillDays == 0 {
		c.Sync.BackfillDays = 3
	}

	key, err := hex.DecodeString(c.Security.EncryptionKey)
	if err != nil || len(key) != 32 {
		return fmt.Errorf("security.encryption_key must be 64 hex chars (32 bytes); generate with: openssl rand -hex 32")
	}

	return nil
}
