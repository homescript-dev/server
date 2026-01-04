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

// HomeAssistantDiscovery represents Home Assistant MQTT Discovery config
// https://www.home-assistant.io/integrations/mqtt#mqtt-discovery
type HomeAssistantDiscovery struct {
	Name              string               `json:"name"`
	UniqueID          string               `json:"unique_id"`
	StateTopic        string               `json:"state_topic"`
	CommandTopic      string               `json:"command_topic,omitempty"`
	AvailabilityTopic string               `json:"availability_topic,omitempty"`
	DeviceClass       string               `json:"device_class,omitempty"`
	UnitOfMeasurement string               `json:"unit_of_measurement,omitempty"`
	ValueTemplate     string               `json:"value_template,omitempty"`
	Device            *HomeAssistantDevice `json:"device,omitempty"`
	PayloadOn         string               `json:"payload_on,omitempty"`
	PayloadOff        string               `json:"payload_off,omitempty"`
	StateOn           string               `json:"state_on,omitempty"`
	StateOff          string               `json:"state_off,omitempty"`
	Icon              string               `json:"icon,omitempty"`
	EntityCategory    string               `json:"entity_category,omitempty"`

	// Climate-specific topics (https://www.home-assistant.io/integrations/climate.mqtt/)
	CurrentTemperatureTopic string `json:"current_temperature_topic,omitempty"`
	TemperatureStateTopic   string `json:"temperature_state_topic,omitempty"`
	TemperatureCommandTopic string `json:"temperature_command_topic,omitempty"`
	ModeStateTopic          string `json:"mode_state_topic,omitempty"`
	ModeCommandTopic        string `json:"mode_command_topic,omitempty"`
	FanModeStateTopic       string `json:"fan_mode_state_topic,omitempty"`
	FanModeCommandTopic     string `json:"fan_mode_command_topic,omitempty"`
	ActionTopic             string `json:"action_topic,omitempty"`

	// Cover-specific topics
	PositionTopic    string `json:"position_topic,omitempty"`
	SetPositionTopic string `json:"set_position_topic,omitempty"`
	TiltStatusTopic  string `json:"tilt_status_topic,omitempty"`
	TiltCommandTopic string `json:"tilt_command_topic,omitempty"`

	// Light-specific topics
	BrightnessStateTopic   string `json:"brightness_state_topic,omitempty"`
	BrightnessCommandTopic string `json:"brightness_command_topic,omitempty"`
	ColorModeStateTopic    string `json:"color_mode_state_topic,omitempty"`
	RGBStateTopic          string `json:"rgb_state_topic,omitempty"`
	RGBCommandTopic        string `json:"rgb_command_topic,omitempty"`

	// Fan-specific topics
	PercentageStateTopic    string `json:"percentage_state_topic,omitempty"`
	PercentageCommandTopic  string `json:"percentage_command_topic,omitempty"`
	PresetModeStateTopic    string `json:"preset_mode_state_topic,omitempty"`
	PresetModeCommandTopic  string `json:"preset_mode_command_topic,omitempty"`
	OscillationStateTopic   string `json:"oscillation_state_topic,omitempty"`
	OscillationCommandTopic string `json:"oscillation_command_topic,omitempty"`

	// Humidifier-specific topics
	TargetHumidityStateTopic   string `json:"target_humidity_state_topic,omitempty"`
	TargetHumidityCommandTopic string `json:"target_humidity_command_topic,omitempty"`
	CurrentHumidityTopic       string `json:"current_humidity_topic,omitempty"`

	// Additional fields that may be present
	Extra map[string]interface{} `json:"-"`
}

// HomeAssistantDevice represents device info in HA discovery
type HomeAssistantDevice struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Model        string   `json:"model,omitempty"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	SWVersion    string   `json:"sw_version,omitempty"`
}

// FrigateCameraStats represents stats for a single camera
type FrigateCameraStats struct {
	CameraFPS    float64 `json:"camera_fps"`
	DetectionFPS float64 `json:"detection_fps"`
	ProcessFPS   float64 `json:"process_fps"`
	SkippedFPS   float64 `json:"skipped_fps"`
}
