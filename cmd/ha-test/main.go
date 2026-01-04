package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	broker := flag.String("broker", "tcp://192.168.1.47:1883", "MQTT broker URL")
	action := flag.String("action", "publish", "Action: publish or subscribe")
	flag.Parse()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(*broker)
	opts.SetClientID("ha_test_tool")

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect: %v", token.Error())
	}
	defer client.Disconnect(250)

	if *action == "publish" {
		// Publish test HA discovery message
		topic := "homeassistant/sensor/test_sensor/temperature/config"
		payload := `{
  "name": "Test Temperature Sensor",
  "unique_id": "test_sensor_temp_001",
  "state_topic": "test/sensor/temperature/state",
  "unit_of_measurement": "°C",
  "device_class": "temperature",
  "device": {
    "identifiers": ["test_sensor_001"],
    "name": "Test ESP32 Sensor",
    "model": "ESP32-DevKit",
    "manufacturer": "Espressif"
  }
}`

		fmt.Printf("Publishing to %s...\n", topic)
		token := client.Publish(topic, 1, true, payload)
		token.Wait()
		if token.Error() != nil {
			log.Fatalf("Failed to publish: %v", token.Error())
		}
		fmt.Println("✓ Published successfully (retained)")

		// Also publish a test state
		stateToken := client.Publish("test/sensor/temperature/state", 0, false, "23.5")
		stateToken.Wait()
		fmt.Println("✓ Published test state: 23.5°C")

	} else if *action == "subscribe" {
		// Subscribe to HA discovery messages
		topic := "homeassistant/#"
		fmt.Printf("Subscribing to %s...\n", topic)

		client.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			fmt.Printf("\n[%s]\n%s\n", msg.Topic(), string(msg.Payload()))
		})

		fmt.Println("Listening for HA discovery messages... (Press Ctrl+C to stop)")
		time.Sleep(30 * time.Second)
	} else if *action == "dump" {
		// Subscribe and collect retained messages
		topic := "homeassistant/#"
		fmt.Printf("Dumping retained messages from %s...\n", topic)

		type Message struct {
			Topic   string
			Payload string
		}
		messages := []Message{}
		var mutex sync.Mutex

		client.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			mutex.Lock()
			messages = append(messages, Message{
				Topic:   msg.Topic(),
				Payload: string(msg.Payload()),
			})
			mutex.Unlock()
		})

		// Wait for messages to arrive
		time.Sleep(2 * time.Second)

		mutex.Lock()
		count := len(messages)
		for i, msg := range messages {
			fmt.Printf("\n=== Message %d/%d ===\n", i+1, count)
			fmt.Printf("Topic: %s\n", msg.Topic)
			fmt.Printf("Payload:\n%s\n", msg.Payload)
		}
		mutex.Unlock()

		fmt.Printf("\nReceived %d retained message(s)\n", count)
	} else {
		fmt.Fprintf(os.Stderr, "Unknown action: %s\n", *action)
		os.Exit(1)
	}
}
