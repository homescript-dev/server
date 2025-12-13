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

// Manager manages smart home devices
type Manager struct {
	client  mqtt.Client
	devices map[string]*types.Device
	states  map[string]map[string]interface{}
	mu      sync.RWMutex
}

// New creates a new device manager
func New(client mqtt.Client, devices []*types.Device) *Manager {
	m := &Manager{
		client:  client,
		devices: make(map[string]*types.Device),
		states:  make(map[string]map[string]interface{}),
	}

	for _, dev := range devices {
		m.devices[dev.ID] = dev
		m.states[dev.ID] = make(map[string]interface{})
	}

	return m
}

// SetClient updates the MQTT client reference
func (m *Manager) SetClient(client mqtt.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.client = client
}

// Get retrieves current state of a device
func (m *Manager) Get(id string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.states[id]
	if !ok {
		return nil, fmt.Errorf("device not found: %s", id)
	}

	// Return a copy
	result := make(map[string]interface{})
	for k, v := range state {
		result[k] = v
	}
	return result, nil
}

// Set updates device state by publishing to MQTT
func (m *Manager) Set(id string, attrs map[string]interface{}) error {
	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("device not found: %s", id)
	}

	// Check MQTT connection status
	if !m.client.IsConnected() {
		logger.Warn("MQTT client not connected when trying to set device %s", id)
		return fmt.Errorf("MQTT client not connected")
	}

	// Publish to command topic
	payload, err := json.Marshal(attrs)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	logger.Debug("Publishing to %s: %s", dev.MQTT.CommandTopic, string(payload))

	token := m.client.Publish(dev.MQTT.CommandTopic, 0, false, payload)

	// Wait with timeout
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("publish timeout after 5 seconds")
	}

	if token.Error() != nil {
		return fmt.Errorf("failed to publish: %w", token.Error())
	}

	logger.Debug("Successfully set device %s: %v", id, attrs)
	return nil
}

// Call executes an action on a device
func (m *Manager) Call(id string, action string, params map[string]interface{}) error {
	// Convert action to state change
	switch action {
	case "turn_on":
		return m.Set(id, map[string]interface{}{"state": "ON"})
	case "turn_off":
		return m.Set(id, map[string]interface{}{"state": "OFF"})
	case "toggle":
		state, err := m.Get(id)
		if err != nil {
			return err
		}
		currentState, ok := state["state"].(string)
		if !ok {
			currentState = "OFF"
		}
		newState := "ON"
		if currentState == "ON" {
			newState = "OFF"
		}
		return m.Set(id, map[string]interface{}{"state": newState})
	default:
		// Generic action - merge params
		if params == nil {
			params = make(map[string]interface{})
		}
		params["action"] = action
		return m.Set(id, params)
	}
}

// UpdateState updates the cached state of a device
func (m *Manager) UpdateState(id string, state map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.devices[id]; !ok {
		return
	}

	if m.states[id] == nil {
		m.states[id] = make(map[string]interface{})
	}

	for k, v := range state {
		m.states[id][k] = v
	}
}

// GetDevice retrieves device configuration
func (m *Manager) GetDevice(id string) (*types.Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dev, ok := m.devices[id]
	return dev, ok
}

// ListDevices returns all devices
func (m *Manager) ListDevices() []*types.Device {
	m.mu.RLock()
	defer m.mu.RUnlock()

	devices := make([]*types.Device, 0, len(m.devices))
	for _, dev := range m.devices {
		devices = append(devices, dev)
	}
	return devices
}
