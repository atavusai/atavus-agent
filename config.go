package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// Config holds the agent's persistent settings
type Config struct {
	ServerURL  string `json:"server_url"`
	AuthToken  string `json:"auth_token"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`

	// Sandbox settings
	AllowedPaths    []string `json:"allowed_paths"`
	BlockedPaths    []string `json:"blocked_paths"`
	MaxFileSizeMB   int      `json:"max_file_size_mb"`
	ConfirmDeletes  bool     `json:"confirm_deletes"`

	// Connection
	ReconnectMaxSec int `json:"reconnect_max_sec"`
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ServerURL:       "",
		AllowedPaths:    []string{"*"}, // Allow ALL paths by default
		BlockedPaths:    []string{},
		MaxFileSizeMB:   50,
		ConfirmDeletes:  true,
		ReconnectMaxSec: 60,
	}
}

// LoadConfig reads config from file, returns defaults if not found
func LoadConfig(path string) *Config {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}

	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		log.Printf("Config parse error, using defaults: %v", err)
		return cfg
	}

	// Merge: keep defaults for missing fields
	if fileCfg.ServerURL != "" {
		cfg.ServerURL = fileCfg.ServerURL
	}
	if fileCfg.AuthToken != "" {
		cfg.AuthToken = fileCfg.AuthToken
	}
	if fileCfg.DeviceID != "" {
		cfg.DeviceID = fileCfg.DeviceID
	}
	if fileCfg.DeviceName != "" {
		cfg.DeviceName = fileCfg.DeviceName
	}
	if len(fileCfg.AllowedPaths) > 0 {
		cfg.AllowedPaths = fileCfg.AllowedPaths
	}
	if len(fileCfg.BlockedPaths) > 0 {
		cfg.BlockedPaths = fileCfg.BlockedPaths
	}
	if fileCfg.MaxFileSizeMB > 0 {
		cfg.MaxFileSizeMB = fileCfg.MaxFileSizeMB
	}
	if fileCfg.ReconnectMaxSec > 0 {
		cfg.ReconnectMaxSec = fileCfg.ReconnectMaxSec
	}
	cfg.ConfirmDeletes = fileCfg.ConfirmDeletes

	return cfg
}

// Save writes config to file
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
