package types

import "time"

// Device represents a smart home device
type Device struct {
	ID         string     `yaml:"id"`
	Name       string     `yaml:"name"`
	Type       string     `yaml:"type"`
	Model      string     `yaml:"model,omitempty"`
	Vendor     string     `yaml:"vendor,omitempty"`
	Attributes []string   `yaml:"attributes"`
	Actions    []string   `yaml:"actions"`
	MQTT       MQTTConfig `yaml:"mqtt"`
}

// MQTTConfig holds MQTT-specific configuration
type MQTTConfig struct {
	StateTopic   string `yaml:"state_topic"`
	CommandTopic string `yaml:"command_topic"`
}

// DevicesConfig is the root configuration structure
type DevicesConfig struct {
	Devices   []*Device `yaml:"devices"`
	Generated time.Time `yaml:"generated,omitempty"`
}

// Event represents an event in the system
type Event struct {
	Source    string                 // "mqtt", "time", "device", "state"
	Type      string                 // event type
	Device    string                 // device ID (if applicable)
	Attribute string                 // attribute name (if applicable)
	Topic     string                 // MQTT topic (if applicable)
	Data      map[string]interface{} // event payload
	Timestamp time.Time
}

// Zigbee2MQTTDevice represents a device from Zigbee2MQTT
type Zigbee2MQTTDevice struct {
	IEEEAddress  string                 `json:"ieee_address"`
	Type         string                 `json:"type"`
	FriendlyName string                 `json:"friendly_name"`
	Definition   *Zigbee2MQTTDefinition `json:"definition"`
}

// Zigbee2MQTTDefinition contains device capabilities
type Zigbee2MQTTDefinition struct {
	Model       string              `json:"model"`
	Vendor      string              `json:"vendor"`
	Description string              `json:"description"`
	Exposes     []Zigbee2MQTTExpose `json:"exposes"`
}

// Zigbee2MQTTExpose represents a device capability
type Zigbee2MQTTExpose struct {
	Type     string               `json:"type"`
	Features []Zigbee2MQTTFeature `json:"features"`
	Property string               `json:"property"`
	Name     string               `json:"name"`
}

// Zigbee2MQTTFeature represents a specific feature of a capability
type Zigbee2MQTTFeature struct {
	Name     string   `json:"name"`
	Property string   `json:"property"`
	Values   []string `json:"values"`
	ValueMin *int     `json:"value_min"`
	ValueMax *int     `json:"value_max"`
}

// FrigateStats represents Frigate statistics message
type FrigateStats struct {
	Cameras map[string]FrigateCameraStats `json:"cameras"`
}

// FrigateCameraActivity represents camera activity message (instant response to frigate/onConnect)
type FrigateCameraActivity map[string]FrigateCameraActivityInfo

// FrigateCameraActivityInfo contains camera activity details
type FrigateCameraActivityInfo struct {
	Motion  bool                `json:"motion"`
	Objects []string            `json:"objects"`
	Config  FrigateCameraConfig `json:"config"`
}

// FrigateCameraConfig contains camera configuration
type FrigateCameraConfig struct {
	Detect                 bool `json:"detect"`
	Enabled                bool `json:"enabled"`
	Snapshots              bool `json:"snapshots"`
	Record                 bool `json:"record"`
	Audio                  bool `json:"audio"`
	Notifications          bool `json:"notifications"`
	NotificationsSuspended int  `json:"notifications_suspended"`
	Autotracking           bool `json:"autotracking"`
	Alerts                 bool `json:"alerts"`
	Detections             bool `json:"detections"`
}

// FrigateCameraStats represents stats for a single camera
type FrigateCameraStats struct {
	CameraFPS    float64 `json:"camera_fps"`
	DetectionFPS float64 `json:"detection_fps"`
	ProcessFPS   float64 `json:"process_fps"`
	SkippedFPS   float64 `json:"skipped_fps"`
}
