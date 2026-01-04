package devices

import (
	"encoding/json"
	"fmt"
	"homescript-server/internal/logger"
	"homescript-server/internal/types"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// HADeviceManager manages Home Assistant devices with their complex topic mappings
type HADeviceManager struct {
	client  mqtt.Client
	configs map[string]*types.HomeAssistantDiscovery // deviceID -> HA config
	mu      sync.RWMutex
}

// NewHADeviceManager creates a new HA device manager
func NewHADeviceManager(client mqtt.Client) *HADeviceManager {
	return &HADeviceManager{
		client:  client,
		configs: make(map[string]*types.HomeAssistantDiscovery),
	}
}

// SetClient updates the MQTT client reference
func (h *HADeviceManager) SetClient(client mqtt.Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.client = client
}

// RegisterDevice registers an HA device with its config
func (h *HADeviceManager) RegisterDevice(deviceID string, config *types.HomeAssistantDiscovery) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configs[deviceID] = config
	logger.Debug("Registered HA device config for %s", deviceID)
}

// IsHADevice checks if a device is an HA device
func (h *HADeviceManager) IsHADevice(deviceID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.configs[deviceID]
	return ok
}

// Set publishes commands to HA device using appropriate command topics
func (h *HADeviceManager) Set(deviceID string, attrs map[string]interface{}) error {
	h.mu.RLock()
	config, ok := h.configs[deviceID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("HA device not registered: %s", deviceID)
	}

	if !h.client.IsConnected() {
		return fmt.Errorf("MQTT client not connected")
	}

	for attr, value := range attrs {
		topic, payload := h.getCommandTopicAndPayload(config, attr, value)

		if topic == "" {
			logger.Warn("No command topic for %s.%s", deviceID, attr)
			continue
		}

		logger.Debug("Publishing to HA device %s: %s = %s", deviceID, topic, string(payload))

		token := h.client.Publish(topic, 0, false, payload)
		if !token.WaitTimeout(5 * time.Second) {
			return fmt.Errorf("publish timeout for %s.%s", deviceID, attr)
		}
		if token.Error() != nil {
			return fmt.Errorf("failed to publish %s.%s: %w", deviceID, attr, token.Error())
		}
	}

	logger.Debug("Successfully set HA device %s: %v", deviceID, attrs)
	return nil
}

// getCommandTopicAndPayload returns appropriate topic and payload for an attribute
func (h *HADeviceManager) getCommandTopicAndPayload(config *types.HomeAssistantDiscovery, attr string, value interface{}) (string, []byte) {
	var topic string
	var payload []byte

	switch attr {
	// Climate attributes
	case "temperature", "target_temperature":
		topic = config.TemperatureCommandTopic
		payload = []byte(fmt.Sprintf("%v", value))

	case "hvac_mode", "mode":
		topic = config.ModeCommandTopic
		payload = []byte(fmt.Sprintf("%v", value))

	case "fan_mode":
		topic = config.FanModeCommandTopic
		payload = []byte(fmt.Sprintf("%v", value))

	case "preset_mode", "preset":
		topic = config.PresetModeCommandTopic
		payload = []byte(fmt.Sprintf("%v", value))

	// Cover attributes
	case "position":
		topic = config.SetPositionTopic
		payload = []byte(fmt.Sprintf("%v", value))

	case "tilt":
		topic = config.TiltCommandTopic
		payload = []byte(fmt.Sprintf("%v", value))

	// Light attributes
	case "brightness":
		topic = config.BrightnessCommandTopic
		payload = []byte(fmt.Sprintf("%v", value))

	case "rgb_color", "color":
		topic = config.RGBCommandTopic
		if jsonVal, err := json.Marshal(value); err == nil {
			payload = jsonVal
		} else {
			payload = []byte(fmt.Sprintf("%v", value))
		}

	// Fan attributes
	case "percentage":
		topic = config.PercentageCommandTopic
		payload = []byte(fmt.Sprintf("%v", value))

	case "oscillating", "oscillate":
		topic = config.OscillationCommandTopic
		if b, ok := value.(bool); ok {
			if b {
				payload = []byte("ON")
			} else {
				payload = []byte("OFF")
			}
		} else {
			payload = []byte(fmt.Sprintf("%v", value))
		}

	// Humidifier attributes
	case "humidity", "target_humidity":
		topic = config.TargetHumidityCommandTopic
		payload = []byte(fmt.Sprintf("%v", value))

	// Generic state/command
	case "state":
		topic = config.CommandTopic
		if b, ok := value.(bool); ok {
			if b {
				payload = []byte(config.PayloadOn)
				if payload == nil || len(payload) == 0 {
					payload = []byte("ON")
				}
			} else {
				payload = []byte(config.PayloadOff)
				if payload == nil || len(payload) == 0 {
					payload = []byte("OFF")
				}
			}
		} else {
			payload = []byte(fmt.Sprintf("%v", value))
		}

	case "command":
		topic = config.CommandTopic
		payload = []byte(fmt.Sprintf("%v", value))

	default:
		// Unknown attribute - use generic command topic with JSON
		topic = config.CommandTopic
		if topic != "" {
			if jsonVal, err := json.Marshal(map[string]interface{}{attr: value}); err == nil {
				payload = jsonVal
			} else {
				payload = []byte(fmt.Sprintf("%v", value))
			}
		}
	}

	return topic, payload
}
