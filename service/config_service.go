package service

import (
	"Mist/config"
	"fmt"
)

// ConfigService handles configuration-related operations
type ConfigService struct{}

// NewConfigService creates a new ConfigService instance
func NewConfigService() *ConfigService {
	return &ConfigService{}
}

// AppConfig represents application configuration
type AppConfig struct {
	Provider string
	APIKey   string
	APIURL   string
	Model    string
}

// GetConfig retrieves the current configuration
func (s *ConfigService) GetConfig() (*AppConfig, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("configuration not loaded")
	}

	return &AppConfig{
		Provider: cfg.Provider,
		APIKey:   cfg.APIKey,
		APIURL:   cfg.APIURL,
		Model:    cfg.Model,
	}, nil
}

// SaveConfig saves the configuration
func (s *ConfigService) SaveConfig(cfg *AppConfig) error {
	appCfg := &config.AppConfig{
		Provider: cfg.Provider,
		APIKey:   cfg.APIKey,
		APIURL:   cfg.APIURL,
		Model:    cfg.Model,
	}

	return config.SaveConfig(appCfg)
}
