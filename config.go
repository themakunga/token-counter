package main

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Credentials is loaded from ~/.token-counter.yaml
type Credentials struct {
	Agents map[string]struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"agents"`
}

// Config is loaded from ~/.config/token-counter/config.yaml
type Config struct {
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	Agents          []AgentConfig `yaml:"agents"`
}

type AgentConfig struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Enabled bool   `yaml:"enabled"`
	Limits  Limits `yaml:"limits"`
}

type Limits struct {
	Hourly  int64 `yaml:"hourly"`
	Daily   int64 `yaml:"daily"`
	Weekly  int64 `yaml:"weekly"`
	Monthly int64 `yaml:"monthly"`
}

var defaultConfig = Config{
	RefreshInterval: 30 * time.Second,
	Agents: []AgentConfig{
		{
			ID: "claude", Name: "Claude Code", Enabled: true,
			Limits: Limits{Hourly: 100_000, Daily: 1_000_000, Weekly: 5_000_000, Monthly: 20_000_000},
		},
		{
			ID: "openai", Name: "OpenAI / Codex", Enabled: true,
			Limits: Limits{Hourly: 50_000, Daily: 500_000, Weekly: 2_000_000, Monthly: 8_000_000},
		},
	},
}

func loadConfig() (*Config, error) {
	path := filepath.Join(os.Getenv("HOME"), ".config", "token-counter", "config.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := defaultConfig
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := defaultConfig // start from defaults so missing fields keep their values
	return &cfg, yaml.Unmarshal(data, &cfg)
}

func loadCredentials() (*Credentials, error) {
	path := filepath.Join(os.Getenv("HOME"), ".token-counter.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Credentials{}, nil
	}
	if err != nil {
		return nil, err
	}
	var creds Credentials
	return &creds, yaml.Unmarshal(data, &creds)
}
