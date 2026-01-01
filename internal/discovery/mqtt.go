package discovery

import (
	"encoding/json"
	"fmt"
	"homescript-server/internal/logger"
	"homescript-server/internal/types"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTDiscovery handles device discovery from MQTT
type MQTTDiscovery struct {
	client          mqtt.Client
	devices         map[string]*types.Device
	mu              sync.RWMutex
	onChange        func([]*types.Device)
	zigbeeReceived  bool
	frigateReceived bool
}

// New creates a new MQTTDiscovery instance
func New(client mqtt.Client) *MQTTDiscovery {
	return &MQTTDiscovery{
		client:  client,
		devices: make(map[string]*types.Device),
	}
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

	logger.Info("Discovery complete: found %d device(s)", deviceCount)

	return d.GetDevices()
}

// GetDevices returns the current list of discovered devices
func (d *MQTTDiscovery) GetDevices() []*types.Device {
	d.mu.RLock()
	defer d.mu.RUnlock()

	devices := make([]*types.Device, 0, len(d.devices))
	for _, dev := range d.devices {
		devices = append(devices, dev)
	}
	return devices
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
	dev := &types.Device{
		ID:         sanitizeID(z2m.FriendlyName),
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
	deviceID := "camera_" + sanitizeID(cameraName)

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
		// Add "camera_" prefix to avoid conflicts with other devices
		deviceID := "camera_" + sanitizeID(cameraName)

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
		// Add "camera_" prefix to avoid conflicts with other devices
		deviceID := "camera_" + sanitizeID(cameraName)

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
