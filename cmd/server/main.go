package main

import (
	"homescript-server/internal/config"
	"homescript-server/internal/devices"
	"homescript-server/internal/discovery"
	"homescript-server/internal/events"
	"homescript-server/internal/executor"
	"homescript-server/internal/geolocation"
	"homescript-server/internal/logger"
	"homescript-server/internal/mqtt"
	"homescript-server/internal/scaffold"
	"homescript-server/internal/scheduler"
	"homescript-server/internal/storage"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	configPath = "./config"
	dbPath     = "./data/state.db"
	mqttBroker = "tcp://localhost:1883"
	mqttUser   = ""
	mqttPass   = ""
	logLevel   = "error"
	latitude   = 0.0
	longitude  = 0.0
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "homescript-server",
		Short: "Smart home automation server with Lua scripting",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Initialize logger based on flag value
			level, err := logger.ParseLevel(logLevel)
			if err != nil {
				level = logger.ERROR
				log.Printf("Invalid log level '%s', using ERROR", logLevel)
			}
			logger.Init(level, true) // true = use colors
		},
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", configPath, "Configuration directory path")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", dbPath, "Database file path")
	rootCmd.PersistentFlags().StringVar(&mqttBroker, "mqtt-broker", mqttBroker, "MQTT broker URL (tcp://host:port)")
	rootCmd.PersistentFlags().StringVar(&mqttUser, "mqtt-user", mqttUser, "MQTT username")
	rootCmd.PersistentFlags().StringVar(&mqttPass, "mqtt-pass", mqttPass, "MQTT password")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", logLevel, "Log level (debug, info, warn, error, critical)")
	rootCmd.PersistentFlags().Float64Var(&latitude, "latitude", latitude, "Latitude for sunrise/sunset (auto-detected if not set)")
	rootCmd.PersistentFlags().Float64Var(&longitude, "longitude", longitude, "Longitude for sunrise/sunset (auto-detected if not set)")

	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(discoverCmd())

	if err := rootCmd.Execute(); err != nil {
		logger.Critical("Fatal error: %v", err)
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the smart home server",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runServer(); err != nil {
				logger.Critical("Server error: %v", err)
				os.Exit(1)
			}
		},
	}
}

func discoverCmd() *cobra.Command {
	var timeout int

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover devices and generate configuration",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runDiscovery(time.Duration(timeout) * time.Second); err != nil {
				logger.Critical("Discovery error: %v", err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().IntVar(&timeout, "timeout", 15, "Discovery timeout in seconds")
	return cmd
}

func runDiscovery(timeout time.Duration) error {
	logger.Info("Starting device discovery...")

	// Connect to MQTT for discovery
	cfg := mqtt.Config{
		Broker:   mqttBroker,
		ClientID: "homescript-discovery",
		Username: mqttUser,
		Password: mqttPass,
	}

	mqttClient, err := mqtt.NewClient(cfg, nil, nil)
	if err != nil {
		return err
	}
	defer mqttClient.Disconnect()

	// Give MQTT connection time to stabilize
	logger.Debug("Waiting for MQTT connection to stabilize...")
	time.Sleep(2 * time.Second)

	// Run discovery
	disc := discovery.New(mqttClient.GetInternalClient())
	discoveredDevices := disc.Discover(timeout)

	if len(discoveredDevices) == 0 {
		logger.Warn("No devices discovered")
		return nil
	}

	logger.Info("Discovered %d device(s)", len(discoveredDevices))

	// Generate devices.yaml
	devicesYAMLPath := configPath + "/devices/devices.yaml"
	if err := config.GenerateDevicesYAML(discoveredDevices, devicesYAMLPath); err != nil {
		return err
	}
	logger.Info("Generated: %s", devicesYAMLPath)

	// Generate script scaffolds
	if err := scaffold.GenerateScaffolds(discoveredDevices, configPath); err != nil {
		return err
	}
	logger.Info("Generated script scaffolds")

	logger.Info("Discovery complete!")
	return nil
}

func runServer() error {
	logger.Info("Starting Smart Home Server...")

	// Load device configuration
	devicesYAMLPath := configPath + "/devices/devices.yaml"
	deviceConfig, err := config.LoadDevicesYAML(devicesYAMLPath)
	if err != nil {
		logger.Error("Failed to load devices config: %v", err)
		logger.Info("Run 'homescript-server discover' first to generate configuration")
		return err
	}
	logger.Info("Loaded %d device(s)", len(deviceConfig.Devices))

	// Initialize storage
	store, err := storage.New(dbPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("Error closing storage: %v", err)
		}
	}()
	logger.Debug("Storage initialized")

	// Connect to MQTT first (without router/deviceManager)
	cfg := mqtt.Config{
		Broker:   mqttBroker,
		ClientID: "homescript-server-" + time.Now().Format("20060102150405"),
		Username: mqttUser,
		Password: mqttPass,
	}

	// Create MQTT client without router (will be set later)
	mqttClient, err := mqtt.NewClient(cfg, nil, nil)
	if err != nil {
		return err
	}
	defer mqttClient.Disconnect()
	logger.Info("MQTT connected")

	// Initialize device manager with MQTT client
	deviceManager := devices.New(mqttClient.GetInternalClient(), deviceConfig.Devices)

	// Initialize executor with device manager and storage
	exec := executor.New(store, deviceManager, configPath)
	pool := executor.NewPool(exec, 10, 100)
	pool.Start()
	defer pool.Stop()
	logger.Debug("Worker pool started with 10 workers")

	// Initialize event router with worker pool
	router := events.New(configPath, pool)
	logger.Debug("Event router initialized")

	// Recreate MQTT client with router and device manager
	mqttClient.Disconnect()
	mqttClient, err = mqtt.NewClient(cfg, router, deviceManager)
	if err != nil {
		return err
	}
	defer mqttClient.Disconnect()

	// Update device manager's MQTT client to use the new connected one
	deviceManager.SetClient(mqttClient.GetInternalClient())

	logger.Debug("MQTT reconnected with event routing")

	// Subscribe to device topics
	if err := mqttClient.SubscribeToDevices(); err != nil {
		return err
	}

	// Auto-detect location if coordinates not specified
	schedulerLatitude := latitude
	schedulerLongitude := longitude

	if latitude == 0.0 && longitude == 0.0 {
		logger.Info("No coordinates specified, attempting to detect location from IP...")
		if loc, err := geolocation.GetLocationByIP(); err == nil {
			schedulerLatitude = loc.Latitude
			schedulerLongitude = loc.Longitude
		} else {
			logger.Warn("Failed to detect location from IP: %v", err)
			logger.Warn("Using default sunrise/sunset times (06:30/18:30)")
		}
	}

	// Initialize and start scheduler for time-based events
	sched := scheduler.New(router, scheduler.Config{
		Latitude:  schedulerLatitude,
		Longitude: schedulerLongitude,
	})

	// Connect executor and scheduler bidirectionally
	exec.SetScheduler(sched)
	sched.SetExecutor(exec)

	sched.Start()
	defer sched.Stop()

	logger.Info("Server is running. Press Ctrl+C to stop.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")
	return nil
}
