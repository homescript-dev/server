package config

import (
	"encoding/json"
	"fmt"
	"homescript-server/internal/types"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// GenerateDevicesYAML creates or updates the devices.yaml file
func GenerateDevicesYAML(devices []*types.Device, path string) error {
	config := types.DevicesConfig{
		Devices:   devices,
		Generated: time.Now(),
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := fmt.Sprintf(`# Auto-generated device configuration
# Generated at: %s
# Edit this file to customize device properties
# Run 'homescript-server discover' to regenerate

`, time.Now().Format(time.RFC3339))

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fullContent := append([]byte(header), data...)
	if err := os.WriteFile(path, fullContent, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// LoadDevicesYAML loads the devices configuration
func LoadDevicesYAML(path string) (*types.DevicesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config types.DevicesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// SaveHAConfigs saves Home Assistant discovery configs to JSON file
func SaveHAConfigs(configs map[string]*types.HomeAssistantDiscovery, path string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal HA configs: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write HA configs: %w", err)
	}

	return nil
}

// LoadHAConfigs loads Home Assistant discovery configs from JSON file
func LoadHAConfigs(path string) (map[string]*types.HomeAssistantDiscovery, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No HA configs file - return empty map
			return make(map[string]*types.HomeAssistantDiscovery), nil
		}
		return nil, fmt.Errorf("failed to read HA configs: %w", err)
	}

	var configs map[string]*types.HomeAssistantDiscovery
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("failed to parse HA configs: %w", err)
	}

	return configs, nil
}
