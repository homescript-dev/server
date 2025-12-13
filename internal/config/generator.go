package config

import (
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
