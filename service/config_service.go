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
	Hotkey   string
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
		Hotkey:   cfg.Hotkey,
	}, nil
}

// SaveConfig saves the configuration
func (s *ConfigService) SaveConfig(cfg *AppConfig) error {
	baseCfg := config.GetConfig()
	mongo := config.MongoConfig{}
	if baseCfg != nil {
		mongo = baseCfg.MongoDB
	}

	appCfg := &config.AppConfig{
		Provider: cfg.Provider,
		APIKey:   cfg.APIKey,
		APIURL:   cfg.APIURL,
		Model:    cfg.Model,
		MongoDB: mongo,
		Hotkey:  cfg.Hotkey,
	}

	return config.SaveConfig(appCfg)
}
