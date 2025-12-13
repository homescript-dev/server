.PHONY: all build run discover clean test docker-build docker-up docker-down help

# Variables
BINARY_NAME=smarthome-server
DOCKER_IMAGE=smarthome-server:latest
CONFIG_DIR=./config
DATA_DIR=./data
MQTT_BROKER=ws://localhost:9001/mqtt

# Default target
all: build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) ./cmd/server
	@echo "Build complete!"

# Run discovery
discover: build
	@echo "Running device discovery..."
	./$(BINARY_NAME) discover \
		--mqtt-broker $(MQTT_BROKER) \
		--config $(CONFIG_DIR) \
		--timeout 30

# Run the server
run: build
	@echo "Starting server..."
	./$(BINARY_NAME) run \
		--mqtt-broker $(MQTT_BROKER) \
		--config $(CONFIG_DIR) \
		--db $(DATA_DIR)/state.db

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -rf $(DATA_DIR)/*.db
	@echo "Clean complete!"

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run ./...

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE) .

# Start Docker Compose services
docker-up:
	@echo "Starting services..."
	docker-compose up -d

# Stop Docker Compose services
docker-down:
	@echo "Stopping services..."
	docker-compose down

# View Docker logs
docker-logs:
	docker-compose logs -f smarthome

# Restart server container
docker-restart:
	docker-compose restart smarthome

# Setup directories
setup:
	@echo "Creating directories..."
	mkdir -p $(CONFIG_DIR)/devices
	mkdir -p $(CONFIG_DIR)/events
	mkdir -p $(DATA_DIR)
	mkdir -p mosquitto/config
	mkdir -p mosquitto/data
	mkdir -p mosquitto/log
	@echo "Setup complete!"

# Initialize Mosquitto config
init-mosquitto: setup
	@echo "Creating Mosquitto configuration..."
	@if [ ! -f mosquitto/config/mosquitto.conf ]; then \
		cat > mosquitto/config/mosquitto.conf <<-EOF; \
		listener 1883\n\
		protocol mqtt\n\
		\n\
		listener 9001\n\
		protocol websockets\n\
		\n\
		allow_anonymous true\n\
		persistence true\n\
		persistence_location /mosquitto/data/\n\
		EOF\
		echo "Mosquitto config created!"; \
	else \
		echo "Mosquitto config already exists"; \
	fi

# Full initialization
init: init-mosquitto docker-build
	@echo "Initialization complete!"

# Development workflow: build and run
dev: build
	@echo "Starting in development mode..."
	./$(BINARY_NAME) run \
		--mqtt-broker $(MQTT_BROKER) \
		--config $(CONFIG_DIR) \
		--db $(DATA_DIR)/state.db

# Help
help:
	@echo "Smart Home Server - Makefile Commands"
	@echo ""
	@echo "Build & Run:"
	@echo "  make build          - Build the binary"
	@echo "  make run            - Build and run the server"
	@echo "  make discover       - Run device discovery"
	@echo "  make dev            - Run in development mode"
	@echo ""
	@echo "Development:"
	@echo "  make test           - Run tests"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Lint code"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make deps           - Install dependencies"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-up      - Start all services"
	@echo "  make docker-down    - Stop all services"
	@echo "  make docker-logs    - View server logs"
	@echo "  make docker-restart - Restart server"
	@echo ""
	@echo "Setup:"
	@echo "  make setup          - Create required directories"
	@echo "  make init-mosquitto - Initialize Mosquitto config"
	@echo "  make init           - Full initialization"
