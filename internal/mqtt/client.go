package mqtt

import (
	"encoding/json"
	"fmt"
	"homescript-server/internal/devices"
	"homescript-server/internal/events"
	"homescript-server/internal/logger"
	"homescript-server/internal/types"
	"io"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Client wraps MQTT client with event routing
type Client struct {
	client        mqtt.Client
	router        *events.Router
	deviceManager *devices.Manager
}

// Config holds MQTT connection configuration
type Config struct {
	Broker   string
	ClientID string
	Username string
	Password string
}

// NewClient creates a new MQTT client
func NewClient(cfg Config, router *events.Router, dm *devices.Manager) (*Client, error) {
	// Disable MQTT library internal logging (we'll handle it ourselves)
	mqtt.ERROR = log.New(io.Discard, "", 0)
	mqtt.CRITICAL = log.New(io.Discard, "", 0)
	mqtt.WARN = log.New(io.Discard, "", 0)

	opts := mqtt.NewClientOptions()

	// Ensure broker URL has tcp:// prefix
	brokerURL := cfg.Broker
	if !strings.HasPrefix(brokerURL, "tcp://") && !strings.HasPrefix(brokerURL, "ssl://") {
		brokerURL = "tcp://" + brokerURL
	}

	logger.Debug("Connecting to MQTT broker at %s...", brokerURL)
	opts.AddBroker(brokerURL)
	opts.SetClientID(cfg.ClientID)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(false) // Disable retry during initial connection
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetWriteTimeout(10 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetKeepAlive(60 * time.Second)

	opts.OnConnect = func(c mqtt.Client) {
		logger.Debug("MQTT connected to %s", brokerURL)
	}

	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		logger.Error("MQTT connection lost: %v", err)
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()

	// Wait for connection with timeout
	if !token.WaitTimeout(15 * time.Second) {
		return nil, fmt.Errorf("connection timeout after 15 seconds")
	}

	if token.Error() != nil {
		return nil, fmt.Errorf("failed to connect to MQTT: %w", token.Error())
	}

	logger.Debug("MQTT connection established successfully")

	mqttClient := &Client{
		client:        client,
		router:        router,
		deviceManager: dm,
	}

	return mqttClient, nil
}

// SubscribeToDevices subscribes to state topics for all devices
func (c *Client) SubscribeToDevices() error {
	devices := c.deviceManager.ListDevices()

	for _, dev := range devices {
		topic := dev.MQTT.StateTopic

		token := c.client.Subscribe(topic, 0, c.makeDeviceHandler(dev))
		if token.Wait() && token.Error() != nil {
			logger.Warn("Failed to subscribe to %s: %v", topic, token.Error())
			continue
		}

		logger.Debug("Subscribed to device: %s (%s)", dev.ID, topic)

		// For Frigate cameras, also subscribe to snapshots
		if dev.Type == "camera" && dev.Vendor == "Frigate NVR" {
			// Subscribe to frigate/CameraName/+/snapshot for all object types
			snapshotTopic := strings.Replace(dev.MQTT.StateTopic, "/+/state", "/+/snapshot", 1)
			token = c.client.Subscribe(snapshotTopic, 0, c.makeDeviceHandler(dev))
			if token.Wait() && token.Error() != nil {
				logger.Warn("Failed to subscribe to snapshots %s: %v", snapshotTopic, token.Error())
			} else {
				logger.Debug("Subscribed to snapshots: %s (%s)", dev.ID, snapshotTopic)
			}
		}
	}

	return nil
}

func (c *Client) makeDeviceHandler(dev *types.Device) mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {
		payload := msg.Payload()
		topic := msg.Topic()

		// Check for JPEG snapshots (binary data starting with 0xFF 0xD8)
		if len(payload) > 2 && payload[0] == 0xFF && payload[1] == 0xD8 {
			// This is a JPEG snapshot from Frigate
			if dev.Type == "camera" && dev.Vendor == "Frigate NVR" {
				c.handleFrigateSnapshot(dev, topic, payload)
			} else {
				logger.Debug("Skipping binary message from %s (size: %d bytes)", dev.ID, len(payload))
			}
			return
		}

		// Skip other large binary data
		if len(payload) > 10000 {
			logger.Debug("Skipping large message from %s (size: %d bytes)", dev.ID, len(payload))
			return
		}

		var state map[string]interface{}

		// Try to parse as JSON first
		if err := json.Unmarshal(payload, &state); err != nil {
			// Not JSON - check if it's a simple value (Frigate publishes ON/OFF, numbers, etc)
			payloadStr := string(payload)

			// Extract attribute name from topic
			// For Frigate: frigate/CameraName/attribute/state -> attribute
			// For Zigbee2MQTT: just use the whole message as-is
			if dev.Type == "camera" && dev.Vendor == "Frigate NVR" {
				// Parse Frigate topic: frigate/CameraName/attribute/state
				parts := strings.Split(topic, "/")
				if len(parts) >= 3 {
					attr := parts[2] // attribute name

					// Create state with single attribute
					state = map[string]interface{}{
						attr: payloadStr,
					}

					logger.Debug("Parsed Frigate simple value: %s = %s", attr, payloadStr)
				} else {
					logger.Debug("Skipping unknown Frigate topic format: %s", topic)
					return
				}
			} else {
				// For non-Frigate devices, skip non-JSON messages
				logger.Debug("Skipping non-JSON message from %s: %v", dev.ID, err)
				return
			}
		}

		// Update device state if device manager is available
		if c.deviceManager != nil {
			c.deviceManager.UpdateState(dev.ID, state)
		}

		// Route events only if router is available
		if c.router == nil {
			return
		}

		// Create events for each changed attribute
		for attr, value := range state {
			// Skip non-attribute fields
			if attr == "linkquality" || attr == "last_seen" {
				continue
			}

			event := &types.Event{
				Source:    "device",
				Type:      "state_change",
				Device:    dev.ID,
				Attribute: attr,
				Topic:     msg.Topic(),
				Data: map[string]interface{}{
					attr: value,
				},
				Timestamp: time.Now(),
			}

			// Copy all state data to event
			for k, v := range state {
				if k != attr {
					event.Data[k] = v
				}
			}

			c.router.RouteEvent(event)
		}

		// Note: We don't create a general MQTT event for device messages
		// to avoid duplicate script execution. Device-specific scripts
		// are already triggered above. If you need raw MQTT handling,
		// subscribe to the topic directly with SubscribeToTopic().
	}
}

// SubscribeToTopic subscribes to a specific MQTT topic
func (c *Client) SubscribeToTopic(topic string) error {
	handler := func(client mqtt.Client, msg mqtt.Message) {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Payload(), &data); err != nil {
			// If not JSON, use raw payload
			data = map[string]interface{}{
				"payload": string(msg.Payload()),
			}
		}

		// Route event only if router is available
		if c.router == nil {
			return
		}

		event := &types.Event{
			Source:    "mqtt",
			Type:      "message",
			Topic:     msg.Topic(),
			Data:      data,
			Timestamp: time.Now(),
		}

		c.router.RouteEvent(event)
	}

	token := c.client.Subscribe(topic, 0, handler)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", topic, token.Error())
	}

	logger.Info("Subscribed to topic: %s", topic)
	return nil
}

// Publish publishes a message to a topic
func (c *Client) Publish(topic string, payload interface{}) error {
	var data []byte
	var err error

	switch v := payload.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
	}

	token := c.client.Publish(topic, 0, false, data)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish: %w", token.Error())
	}

	return nil
}

// handleFrigateSnapshot processes JPEG snapshot from Frigate
func (c *Client) handleFrigateSnapshot(dev *types.Device, topic string, payload []byte) {
	// Parse topic: frigate/CameraName/ObjectType/snapshot
	parts := strings.Split(topic, "/")
	if len(parts) < 4 || parts[3] != "snapshot" {
		logger.Debug("Skipping non-snapshot Frigate binary topic: %s", topic)
		return
	}

	objectType := parts[2] // person, car, dog, etc

	logger.Debug("Received %s snapshot from %s (size: %d bytes)", objectType, dev.ID, len(payload))

	// Route event only if router is available
	if c.router == nil {
		return
	}

	// Create event for snapshot
	event := &types.Event{
		Source:    "device",
		Type:      "snapshot",
		Device:    dev.ID,
		Attribute: objectType, // "person", "car", etc
		Topic:     topic,
		Data: map[string]interface{}{
			"object_type": objectType,
			"snapshot":    payload, // Raw JPEG bytes
			"size":        len(payload),
		},
		Timestamp: time.Now(),
	}

	c.router.RouteEvent(event)
}

// Disconnect closes the MQTT connection
func (c *Client) Disconnect() {
	c.client.Disconnect(250)
	logger.Debug("MQTT disconnected")
}

// GetInternalClient returns the underlying MQTT client
func (c *Client) GetInternalClient() mqtt.Client {
	return c.client
}

// ParseTopicToPath converts MQTT topic to filesystem path
func ParseTopicToPath(topic string) string {
	return strings.ReplaceAll(topic, "/", "_")
}
