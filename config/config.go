package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// MongoConfig MongoDB配置结构
type MongoConfig struct {
	ConnectionString  string `json:"connection_string"`
	DatabaseName      string `json:"database_name"`
	ConnectionTimeout int    `json:"connection_timeout"`
}

// AppConfig 应用配置结构
type AppConfig struct {
	Provider string      `json:"provider"`
	APIKey   string      `json:"api_key"`
	APIURL   string      `json:"api_url"`
	Model    string      `json:"model"`
	MongoDB  MongoConfig `json:"mongodb"`
	Hotkey   string      `json:"hotkey"`
}

var (
	config     *AppConfig
	configPath = "config.json"
)

// LoadConfig 加载配置文件
func LoadConfig() (*AppConfig, error) {

	// 如果配置文件不存在，创建默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := &AppConfig{
			Provider: "custom",
			APIKey:   "",
			APIURL:   "",
			Model:    "",
			MongoDB: MongoConfig{
				ConnectionString:  "mongodb://localhost:27017",
				DatabaseName:      "chat_assistant",
				ConnectionTimeout: 10,
			},
			Hotkey: "ctrl+c",
		}
		if err := SaveConfig(defaultConfig); err != nil {
			return nil, fmt.Errorf("创建默认配置失败: %v", err)
		}
		config = defaultConfig
		return config, nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	config = &cfg
	return config, nil
}

// SaveConfig 保存配置文件
func SaveConfig(cfg *AppConfig) error {

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	config = cfg
	return nil
}

// GetConfig 获取当前配置
func GetConfig() *AppConfig {
	return config
}
