package discovery

import (
	"encoding/json"
	"fmt"
	"homescript-server/internal/devices"
	"homescript-server/internal/logger"
	"homescript-server/internal/types"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTDiscovery handles device discovery from MQTT
type MQTTDiscovery struct {
	client               mqtt.Client
	devices              map[string]*types.Device
	haManager            *devices.HADeviceManager
	mu                   sync.RWMutex
	onChange             func([]*types.Device)
	zigbeeReceived       bool
	frigateReceived      bool
	homeAssistantDevices map[string]*homeAssistantEntity // Track HA entities by topic
}

// homeAssistantEntity tracks a Home Assistant discovered entity
type homeAssistantEntity struct {
	component  string                        // sensor, switch, light, etc.
	objectID   string                        // unique identifier
	config     *types.HomeAssistantDiscovery // discovery config
	deviceID   string                        // our internal device ID
	attributes []string                      // collected attributes for this device
}

// New creates a new MQTTDiscovery instance
func New(client mqtt.Client) *MQTTDiscovery {
	return &MQTTDiscovery{
		client:               client,
		devices:              make(map[string]*types.Device),
		homeAssistantDevices: make(map[string]*homeAssistantEntity),
	}
}

// SetHAManager sets the HA device manager
func (d *MQTTDiscovery) SetHAManager(haManager *devices.HADeviceManager) {
	d.haManager = haManager
}

// SetOnChange sets the callback for when devices change
func (d *MQTTDiscovery) SetOnChange(fn func([]*types.Device)) {
	d.onChange = fn
}

// Start begins listening for device discovery messages
func (d *MQTTDiscovery) Start() error {
	logger.Debug("Starting MQTT discovery subscriptions...")

	// Subscribe to Zigbee2MQTT devices
	logger.Debug("Subscribing to zigbee2mqtt/bridge/devices...")
	token := d.client.Subscribe("zigbee2mqtt/bridge/devices", 0, d.handleDevices)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to zigbee2mqtt: %w", token.Error())
	}

	// Request current device list
	token = d.client.Publish("zigbee2mqtt/bridge/config/devices/get", 0, false, "")
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to request zigbee2mqtt devices: %w", token.Error())
	}

	// Subscribe to Frigate camera activity for instant camera discovery
	logger.Debug("Subscribing to frigate/camera_activity...")
	token = d.client.Subscribe("frigate/camera_activity", 0, d.handleFrigateCameraActivity)
	if token.Wait() && token.Error() != nil {
		logger.Debug("Failed to subscribe to frigate/camera_activity (Frigate not available?): %v", token.Error())
	} else {
		logger.Debug("Successfully subscribed to frigate/camera_activity for camera discovery")

		// Trigger Frigate to send camera_activity immediately
		// According to docs: frigate/onConnect triggers immediate frigate/camera_activity response
		logger.Debug("Publishing to frigate/onConnect to trigger immediate camera_activity...")
		token = d.client.Publish("frigate/onConnect", 0, false, "ON")
		if token.Wait() && token.Error() != nil {
			logger.Debug("Failed to trigger frigate/onConnect: %v", token.Error())
		} else {
			logger.Debug("Successfully triggered frigate/onConnect with 'ON'")
		}
	}

	// Subscribe to Home Assistant MQTT Discovery
	// Support both formats:
	//   - homeassistant/<component>/<object_id>/config (4 parts)
	//   - homeassistant/<component>/<node_id>/<object_id>/config (5 parts)

	// Subscribe to 5-part format
	logger.Debug("Subscribing to homeassistant/+/+/+/config (5-part format)...")
	token = d.client.Subscribe("homeassistant/+/+/+/config", 0, d.handleHomeAssistantDiscovery)
	if token.Wait() && token.Error() != nil {
		logger.Debug("Failed to subscribe to HA discovery 5-part format: %v", token.Error())
	} else {
		logger.Debug("Successfully subscribed to HA discovery (5-part)")
	}

	// Subscribe to 4-part format (simplified)
	logger.Debug("Subscribing to homeassistant/+/+/config (4-part format)...")
	token = d.client.Subscribe("homeassistant/+/+/config", 0, d.handleHomeAssistantDiscovery)
	if token.Wait() && token.Error() != nil {
		logger.Debug("Failed to subscribe to HA discovery 4-part format: %v", token.Error())
	} else {
		logger.Debug("Successfully subscribed to HA discovery (4-part)")

		// Publish birth message to announce our presence
		// This tells HA-aware devices that we're online and ready
		logger.Debug("Publishing birth message to homeassistant/status...")
		token = d.client.Publish("homeassistant/status", 1, true, "online")
		if token.Wait() && token.Error() != nil {
			logger.Debug("Failed to publish birth message: %v", token.Error())
		} else {
			logger.Debug("Successfully published birth message")
		}
	}

	logger.Info("MQTT discovery started")
	return nil
}

// Discover performs a one-time discovery with timeout
func (d *MQTTDiscovery) Discover(timeout time.Duration) []*types.Device {
	if err := d.Start(); err != nil {
		logger.Debug("Failed to start discovery: %v", err)
		return nil
	}

	logger.Debug("Discovering devices (timeout: %v)...", timeout)

	// Wait for Zigbee2MQTT devices (usually responds in 1-2 seconds)
	zigbeeTimeout := 5 * time.Second
	if timeout < zigbeeTimeout {
		zigbeeTimeout = timeout
	}

	deadline := time.Now().Add(zigbeeTimeout)
	for time.Now().Before(deadline) {
		d.mu.RLock()
		received := d.zigbeeReceived
		d.mu.RUnlock()

		if received {
			logger.Debug("Zigbee2MQTT devices received")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Wait a bit for Frigate camera_activity (triggered by frigate/onConnect)
	// frigate/camera_activity is published immediately in response to frigate/onConnect
	// so we only need to wait a few seconds
	frigateTimeout := 5 * time.Second
	remaining := time.Until(time.Now().Add(timeout))
	if remaining > 0 && remaining < frigateTimeout {
		frigateTimeout = remaining
	}

	if frigateTimeout > 0 {
		logger.Debug("Waiting up to %v for Frigate cameras...", frigateTimeout)
		deadline = time.Now().Add(frigateTimeout)
		for time.Now().Before(deadline) {
			d.mu.RLock()
			received := d.frigateReceived
			d.mu.RUnlock()

			if received {
				logger.Debug("Frigate cameras received")
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	d.mu.RLock()
	deviceCount := len(d.devices)
	frigateReceived := d.frigateReceived
	d.mu.RUnlock()

	if !frigateReceived {
		logger.Debug("No Frigate cameras detected (Frigate may not be available)")
	}

	// Wait for Home Assistant discovery messages (retained messages should arrive quickly)
	// HA devices publish their config once with retained flag, so we should receive them
	// within a second or two after subscribing
	haTimeout := 3 * time.Second
	remaining = time.Until(time.Now().Add(timeout))
	if remaining > 0 && remaining < haTimeout {
		haTimeout = remaining
	}

	if haTimeout > 0 {
		logger.Debug("Waiting up to %v for Home Assistant devices...", haTimeout)
		initialHACount := 0
		d.mu.RLock()
		// Count HA devices (those with "ha/" prefix)
		for id := range d.devices {
			if strings.HasPrefix(id, "ha/") {
				initialHACount++
			}
		}
		d.mu.RUnlock()

		deadline = time.Now().Add(haTimeout)
		for time.Now().Before(deadline) {
			d.mu.RLock()
			haCount := 0
			for id := range d.devices {
				if strings.HasPrefix(id, "ha/") {
					haCount++
				}
			}
			d.mu.RUnlock()

			// If we found some HA devices, wait a bit more to see if more arrive
			if haCount > initialHACount {
				time.Sleep(500 * time.Millisecond)
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		d.mu.RLock()
		haCount := 0
		for id := range d.devices {
			if strings.HasPrefix(id, "ha/") {
				haCount++
			}
		}
		d.mu.RUnlock()

		if haCount > 0 {
			logger.Debug("Found %d Home Assistant device(s)", haCount)

			// Wait additional time for state_topic messages to populate attributes
			logger.Debug("Waiting 2s for HA state messages to discover attributes...")
			time.Sleep(2 * time.Second)
		} else {
			logger.Debug("No Home Assistant devices detected")
		}
	}

	d.mu.RLock()
	deviceCount = len(d.devices)
	d.mu.RUnlock()

	logger.Info("Discovery complete: found %d device(s)", deviceCount)

	return d.GetDevices()
}

// GetDevices returns discovered devices
func (d *MQTTDiscovery) GetDevices() []*types.Device {
	d.mu.RLock()
	defer d.mu.RUnlock()

	devices := make([]*types.Device, 0, len(d.devices))
	for _, dev := range d.devices {
		devices = append(devices, dev)
	}
	return devices
}

// GetHAConfigs returns all HA discovery configs (deviceID -> config)
func (d *MQTTDiscovery) GetHAConfigs() map[string]*types.HomeAssistantDiscovery {
	d.mu.RLock()
	defer d.mu.RUnlock()

	configs := make(map[string]*types.HomeAssistantDiscovery)
	for _, entity := range d.homeAssistantDevices {
		configs[entity.deviceID] = entity.config
	}
	return configs
}

func (d *MQTTDiscovery) handleDevices(_ mqtt.Client, msg mqtt.Message) {
	var z2mDevices []types.Zigbee2MQTTDevice
	if err := json.Unmarshal(msg.Payload(), &z2mDevices); err != nil {
		logger.Debug("Failed to parse devices: %v", err)
		return
	}

	d.mu.Lock()
	d.devices = make(map[string]*types.Device)

	for _, z2mDev := range z2mDevices {
		// Skip coordinator and devices without definition
		if z2mDev.Definition == nil || z2mDev.Type == "Coordinator" {
			continue
		}

		dev := d.convertToDevice(z2mDev)
		d.devices[dev.ID] = dev
	}

	d.zigbeeReceived = true
	d.mu.Unlock()

	logger.Debug("Discovered %d Zigbee2MQTT device(s)", len(d.devices))

	if d.onChange != nil {
		d.onChange(d.GetDevices())
	}
}

func (d *MQTTDiscovery) convertToDevice(z2m types.Zigbee2MQTTDevice) *types.Device {
	// Zigbee devices: no prefix (default source) or "zigbee/" if you prefer explicit
	// For simplicity, we'll keep them without prefix as they're the default
	deviceID := sanitizeID(z2m.FriendlyName)

	dev := &types.Device{
		ID:         deviceID,
		Name:       z2m.FriendlyName,
		Model:      z2m.Definition.Model,
		Vendor:     z2m.Definition.Vendor,
		Attributes: make([]string, 0),
		Actions:    make([]string, 0),
		MQTT: types.MQTTConfig{
			StateTopic:   fmt.Sprintf("zigbee2mqtt/%s", z2m.FriendlyName),
			CommandTopic: fmt.Sprintf("zigbee2mqtt/%s/set", z2m.FriendlyName),
		},
	}

	// Extract attributes and actions from exposes
	attrMap := make(map[string]bool)
	for _, expose := range z2m.Definition.Exposes {
		if expose.Type != "" {
			dev.Type = expose.Type
		}

		// Handle direct properties (like battery, linkquality)
		if expose.Property != "" && expose.Property != "state" {
			attrMap[expose.Property] = true
		}

		// Handle features
		for _, feature := range expose.Features {
			if feature.Property != "" {
				attrMap[feature.Property] = true

				// Generate actions for state property
				if feature.Property == "state" {
					dev.Actions = append(dev.Actions, "turn_on", "turn_off", "toggle")
				}
			}
		}
	}

	// Convert map to slice
	for attr := range attrMap {
		dev.Attributes = append(dev.Attributes, attr)
	}

	// Set default type if not found
	if dev.Type == "" {
		dev.Type = "sensor"
	}

	return dev
}

func sanitizeID(name string) string {
	// Replace spaces and special characters with underscores
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "_")
	id = strings.ReplaceAll(id, "-", "_")

	// Remove any remaining non-alphanumeric characters
	result := ""
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			result += string(r)
		}
	}

	return result
}

// createFrigateCameraDevice creates a Device object for a Frigate camera
func createFrigateCameraDevice(cameraName string) *types.Device {
	// Frigate devices: frigate/<camera_name>
	deviceID := "frigate/" + sanitizeID(cameraName)

	return &types.Device{
		ID:     deviceID,
		Name:   cameraName,
		Type:   "camera",
		Model:  "Frigate Camera",
		Vendor: "Frigate NVR",
		Attributes: []string{
			// Configuration attributes (from frigate/{camera_name}/{attribute}/state)
			"enabled",          // Camera enabled state (ON/OFF)
			"detect",           // Detection enabled (ON/OFF)
			"motion",           // Motion detection enabled (ON/OFF)
			"recordings",       // Recordings enabled (ON/OFF)
			"snapshots",        // Snapshots enabled (ON/OFF)
			"audio",            // Audio detection enabled (ON/OFF)
			"improve_contrast", // Improve contrast (ON/OFF)
			"ptz_autotracker",  // PTZ autotracker (ON/OFF)
			// Threshold attributes
			"motion_threshold",    // Motion detection threshold (0-255)
			"motion_contour_area", // Motion contour area threshold
			// Object detection counts (from frigate/{camera_name}/{object_type})
			"person",
			"car",
			"dog",
			"cat",
			"all", // All detected objects count
		},
		Actions: []string{
			// Actions map to frigate/{camera_name}/{action}/set topics
			"enable",              // frigate/{camera_name}/enabled/set
			"disable",             // frigate/{camera_name}/enabled/set
			"detect_on",           // frigate/{camera_name}/detect/set ON
			"detect_off",          // frigate/{camera_name}/detect/set OFF
			"motion_on",           // frigate/{camera_name}/motion/set ON
			"motion_off",          // frigate/{camera_name}/motion/set OFF
			"recordings_on",       // frigate/{camera_name}/recordings/set ON
			"recordings_off",      // frigate/{camera_name}/recordings/set OFF
			"snapshots_on",        // frigate/{camera_name}/snapshots/set ON
			"snapshots_off",       // frigate/{camera_name}/snapshots/set OFF
			"improve_contrast",    // frigate/{camera_name}/improve_contrast/set ON/OFF
			"ptz_autotracker",     // frigate/{camera_name}/ptz_autotracker/set ON/OFF
			"motion_threshold",    // frigate/{camera_name}/motion_threshold/set <value>
			"motion_contour_area", // frigate/{camera_name}/motion_contour_area/set <value>
		},
		MQTT: types.MQTTConfig{
			// State topic pattern for configuration states
			// frigate/{camera_name}/detect/state, frigate/{camera_name}/enabled/state, etc.
			StateTopic: fmt.Sprintf("frigate/%s/#", cameraName),
			// Command topic base - each action constructs its own topic
			CommandTopic: fmt.Sprintf("frigate/%s", cameraName),
		},
	}
}

func (d *MQTTDiscovery) handleFrigateStats(_ mqtt.Client, msg mqtt.Message) {
	logger.Debug("Received Frigate stats message")

	var stats types.FrigateStats
	if err := json.Unmarshal(msg.Payload(), &stats); err != nil {
		logger.Debug("Failed to parse Frigate stats: %v", err)
		return
	}

	logger.Debug("Parsed Frigate stats, found %d camera(s)", len(stats.Cameras))

	d.mu.Lock()
	defer d.mu.Unlock()

	// Convert each camera to a Device
	for cameraName := range stats.Cameras {
		// Frigate devices: frigate/<camera_name>
		deviceID := "frigate/" + sanitizeID(cameraName)

		// Skip if already added
		if _, exists := d.devices[deviceID]; exists {
			logger.Debug("Camera %s already in devices", cameraName)
			continue
		}

		// Create device for camera
		dev := createFrigateCameraDevice(cameraName)
		d.devices[deviceID] = dev
		logger.Debug("Discovered Frigate camera: %s", cameraName)
	}

	d.frigateReceived = true

	if d.onChange != nil {
		d.onChange(d.GetDevices())
	}
}

func (d *MQTTDiscovery) handleFrigateCameraActivity(_ mqtt.Client, msg mqtt.Message) {
	logger.Debug("Received Frigate camera_activity message")

	var cameraActivity types.FrigateCameraActivity
	if err := json.Unmarshal(msg.Payload(), &cameraActivity); err != nil {
		logger.Debug("Failed to parse Frigate camera_activity: %v", err)
		return
	}

	logger.Debug("Parsed Frigate camera_activity, found %d camera(s)", len(cameraActivity))

	d.mu.Lock()
	defer d.mu.Unlock()

	// Convert each camera to a Device
	for cameraName := range cameraActivity {
		// Frigate devices: frigate/<camera_name>
		deviceID := "frigate/" + sanitizeID(cameraName)

		// Skip if already added
		if _, exists := d.devices[deviceID]; exists {
			logger.Debug("Camera %s already in devices", cameraName)
			continue
		}

		// Create device for camera
		dev := createFrigateCameraDevice(cameraName)
		d.devices[deviceID] = dev
		logger.Debug("Discovered Frigate camera: %s", cameraName)
	}

	d.frigateReceived = true

	if d.onChange != nil {
		d.onChange(d.GetDevices())
	}
}

// handleHomeAssistantDiscovery processes Home Assistant MQTT Discovery messages
// Topic formats:
//   - homeassistant/<component>/<object_id>/config (4 parts - simplified)
//   - homeassistant/<component>/<node_id>/<object_id>/config (5 parts - full)
func (d *MQTTDiscovery) handleHomeAssistantDiscovery(_ mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()

	logger.Debug("Received Home Assistant discovery message: %s", topic)

	// Parse topic - support both 4-part and 5-part formats
	parts := strings.Split(topic, "/")

	var component, nodeID, objectID string

	if len(parts) == 4 && parts[0] == "homeassistant" && parts[3] == "config" {
		// Simplified format: homeassistant/<component>/<object_id>/config
		component = parts[1]
		objectID = parts[2]
		nodeID = parts[2] // Use object_id as node_id in simplified format
	} else if len(parts) == 5 && parts[0] == "homeassistant" && parts[4] == "config" {
		// Full format: homeassistant/<component>/<node_id>/<object_id>/config
		component = parts[1]
		nodeID = parts[2]
		objectID = parts[3]
	} else {
		logger.Debug("Invalid HA discovery topic format: %s (expected 4 or 5 parts)", topic)
		return
	}

	// Empty payload means device was removed
	if len(payload) == 0 {
		logger.Debug("HA device removed: %s/%s/%s", component, nodeID, objectID)
		d.removeHomeAssistantDevice(topic)
		return
	}

	// Parse discovery config
	var config types.HomeAssistantDiscovery
	if err := json.Unmarshal(payload, &config); err != nil {
		logger.Debug("Failed to parse HA discovery config: %v", err)
		return
	}

	logger.Debug("Discovered HA entity: %s (name=%s, unique_id=%s)", topic, config.Name, config.UniqueID)

	d.mu.Lock()
	defer d.mu.Unlock()

	// Create internal device ID - prefer unique_id, fallback to object_id/node_id
	var deviceID string
	if config.UniqueID != "" {
		// Use unique_id as device ID (most reliable)
		deviceID = sanitizeID(config.UniqueID)
	} else if config.Device != nil && len(config.Device.Identifiers) > 0 {
		// Use device identifier
		deviceID = sanitizeID(config.Device.Identifiers[0])
	} else if objectID != nodeID {
		// 5-part format - use object_id
		deviceID = sanitizeID(objectID)
	} else {
		// 4-part format - use node_id
		deviceID = sanitizeID(nodeID)
	}

	// Home Assistant devices: ha/<device_id>
	deviceID = "ha/" + deviceID

	// Get appropriate state_topic for this component type
	stateTopic := getHAStateTopic(component, &config)
	// Don't set single command_topic for HA devices - they use multiple topic per attribute
	commandTopic := ""

	// Store entity info (don't track attributes for HA devices - they come dynamically from state_topic)
	entity := &homeAssistantEntity{
		component:  component,
		objectID:   objectID,
		config:     &config,
		deviceID:   deviceID,
		attributes: []string{}, // Empty - HA devices send attributes dynamically
	}
	d.homeAssistantDevices[topic] = entity

	// Create or update device
	dev, exists := d.devices[deviceID]
	if !exists {
		// Create new device
		dev = &types.Device{
			ID:   deviceID,
			Name: config.Name,
			Type: mapHAComponentToType(component),
			MQTT: types.MQTTConfig{
				StateTopic:   stateTopic,
				CommandTopic: commandTopic,
			},
			Attributes: []string{},
			Actions:    []string{},
		}

		if config.Device != nil {
			dev.Model = config.Device.Model
			dev.Vendor = config.Device.Manufacturer
			if dev.Name == "" {
				dev.Name = config.Device.Name
			}
		}

		d.devices[deviceID] = dev
		logger.Debug("Created HA device: %s (type=%s)", deviceID, dev.Type)

		// Register HA config with HADeviceManager for multi-topic command handling
		if d.haManager != nil {
			d.haManager.RegisterDevice(deviceID, &config)
		}
	} else {
		// Update MQTT topics if not set (first entity with topics wins)
		if dev.MQTT.StateTopic == "" && stateTopic != "" {
			dev.MQTT.StateTopic = stateTopic
		}
		if dev.MQTT.CommandTopic == "" && commandTopic != "" {
			dev.MQTT.CommandTopic = commandTopic
		}
	}

	// Don't add attributes for HA devices - they send attributes dynamically in state_topic JSON
	// Scaffold will generate typical attributes for examples

	// Add actions based on THIS entity's component type and config
	actions := getHAActions(component, &config)
	for _, action := range actions {
		if !contains(dev.Actions, action) {
			dev.Actions = append(dev.Actions, action)
		}
	}

	logger.Debug("HA device %s now has %d actions", deviceID, len(dev.Actions))

	if d.onChange != nil {
		d.onChange(d.GetDevices())
	}
}

func (d *MQTTDiscovery) removeHomeAssistantDevice(topic string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entity, exists := d.homeAssistantDevices[topic]
	if !exists {
		return
	}

	delete(d.homeAssistantDevices, topic)

	deviceID := entity.deviceID
	hasOtherEntities := false
	for _, e := range d.homeAssistantDevices {
		if e.deviceID == deviceID {
			hasOtherEntities = true
			break
		}
	}

	if !hasOtherEntities {
		delete(d.devices, deviceID)
		logger.Debug("Removed HA device: %s", deviceID)

		if d.onChange != nil {
			d.onChange(d.GetDevices())
		}
	}
}

func mapHAComponentToType(component string) string {
	switch component {
	// Basic controls
	case "light":
		return "light"
	case "switch":
		return "switch"
	case "button":
		return "button"
	case "scene":
		return "scene"

	// Sensors
	case "binary_sensor":
		return "binary_sensor"
	case "sensor":
		return "sensor"
	case "event":
		return "event"
	case "tag":
		return "tag"

	// Climate & HVAC
	case "climate":
		return "climate"
	case "humidifier":
		return "humidifier"
	case "water_heater":
		return "water_heater"

	// Covers & Valves
	case "cover":
		return "cover"
	case "valve":
		return "valve"

	// Security
	case "alarm_control_panel":
		return "alarm"
	case "lock":
		return "lock"
	case "siren":
		return "siren"

	// Media & Monitoring
	case "camera":
		return "camera"
	case "image":
		return "image"

	// Appliances
	case "fan":
		return "fan"
	case "vacuum":
		return "vacuum"
	case "lawn_mower":
		return "lawn_mower"

	// Input controls
	case "number":
		return "number"
	case "select":
		return "select"
	case "text":
		return "text"

	// Tracking & Updates
	case "device_tracker":
		return "device_tracker"
	case "device_trigger":
		return "device_trigger"
	case "update":
		return "update"

	default:
		return component
	}
}

func getHAActions(component string, config *types.HomeAssistantDiscovery) []string {
	if config.CommandTopic == "" {
		return []string{}
	}

	switch component {
	// Basic controls
	case "light":
		return []string{"turn_on", "turn_off", "toggle"}
	case "switch":
		return []string{"turn_on", "turn_off", "toggle"}
	case "button":
		return []string{"press"}
	case "scene":
		return []string{"turn_on"}

	// Climate & HVAC
	case "climate":
		return []string{"set_temperature", "set_hvac_mode", "set_fan_mode", "set_preset_mode"}
	case "humidifier":
		return []string{"turn_on", "turn_off", "set_humidity", "set_mode"}
	case "water_heater":
		return []string{"set_temperature", "set_operation_mode"}

	// Covers & Valves
	case "cover":
		return []string{"open", "close", "stop", "set_position"}
	case "valve":
		return []string{"open", "close", "stop", "set_position"}

	// Security
	case "alarm_control_panel":
		return []string{"arm_home", "arm_away", "arm_night", "arm_vacation", "arm_custom_bypass", "disarm"}
	case "lock":
		return []string{"lock", "unlock", "open"}
	case "siren":
		return []string{"turn_on", "turn_off"}

	// Appliances
	case "fan":
		return []string{"turn_on", "turn_off", "set_percentage", "set_preset_mode", "oscillate"}
	case "vacuum":
		return []string{"start", "pause", "stop", "return_to_base", "clean_spot", "locate", "set_fan_speed"}
	case "lawn_mower":
		return []string{"start_mowing", "pause", "dock"}

	// Input controls
	case "number":
		return []string{"set_value"}
	case "select":
		return []string{"select_option"}
	case "text":
		return []string{"set_value"}

	// Read-only or automatic components (no actions)
	case "sensor", "binary_sensor", "event", "tag":
		return []string{}
	case "camera", "image":
		return []string{}
	case "device_tracker", "device_trigger", "update":
		return []string{}

	default:
		return []string{}
	}
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// getHAStateTopic returns the appropriate state topic for the component type
func getHAStateTopic(component string, config *types.HomeAssistantDiscovery) string {
	switch component {
	case "climate":
		// For climate, prefer specific topics, fallback to general state_topic
		if config.CurrentTemperatureTopic != "" {
			return config.CurrentTemperatureTopic
		}
		if config.TemperatureStateTopic != "" {
			return config.TemperatureStateTopic
		}
		if config.ModeStateTopic != "" {
			return config.ModeStateTopic
		}
		return config.StateTopic

	case "cover":
		if config.PositionTopic != "" {
			return config.PositionTopic
		}
		return config.StateTopic

	case "light":
		if config.BrightnessStateTopic != "" {
			return config.BrightnessStateTopic
		}
		return config.StateTopic

	case "fan":
		if config.PercentageStateTopic != "" {
			return config.PercentageStateTopic
		}
		return config.StateTopic

	case "humidifier":
		if config.CurrentHumidityTopic != "" {
			return config.CurrentHumidityTopic
		}
		return config.StateTopic

	default:
		return config.StateTopic
	}
}
