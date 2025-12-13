# HomeScript Server

Event-driven automation server for Zigbee2MQTT devices with Lua scripting.

## Features

- 🔍 **Automatic device discovery** from Zigbee2MQTT
- 📝 **Lua-based event scripting** for flexible automation
- 🔄 **MQTT integration** (native TCP on port 1883)
- 💾 **Persistent state storage** using bbolt
- ⚡ **Event-driven architecture** with worker pool
- 🔧 **Hot-reload Lua scripts**

## Quick Start

### 1. Discovery

Discover devices from Zigbee2MQTT and Frigate cameras:

```bash
./homescript-server discover \
  --mqtt-broker tcp://192.168.1.47:1883 \
  --config ./config
```

This will (completes in ~2-3 seconds):
- Connect to your MQTT broker
- Discover all Zigbee2MQTT devices
- Discover Frigate NVR cameras (if available)
- Generate `config/devices/devices.yaml`
- Create Lua script scaffolds in `config/events/`

**Note:** Discovery is fast! Frigate cameras are discovered instantly via `frigate/camera_activity` trigger.

### 2. Run Server

Start the automation server:

```bash
# Coordinates auto-detected from your IP address
./homescript-server run \
  --mqtt-broker tcp://192.168.1.47:1883 \
  --config ./config \
  --db ./data/state.db

# Or specify exact coordinates
./homescript-server run \
  --mqtt-broker tcp://192.168.1.47:1883 \
  --config ./config \
  --db ./data/state.db \
  --latitude 55.7558 \
  --longitude 37.6173
```

**Sunrise/Sunset**: Location is automatically detected from your public IP. Override with `--latitude` and `--longitude` for precise coordinates.

## Configuration

### MQTT Broker

The server uses native MQTT over TCP (default port 1883).

Example Mosquitto configuration (`mosquitto/config/mosquitto.conf`):

```conf
# Standard MQTT over TCP
listener 1883
protocol mqtt
allow_anonymous true

# WebSocket support (optional)
listener 8880
protocol websockets
allow_anonymous true

# Logging
log_dest stdout
log_type all

# Persistence
persistence true
persistence_location /mosquitto/data/
```

### Devices Configuration

Edit `config/devices/devices.yaml` to customize device properties:

```yaml
devices:
  - id: porch
    name: Porch
    type: numeric
    model: ZBMINIL2
    vendor: SONOFF
    attributes:
      - state
      - linkquality
    actions:
      - turn_on
      - turn_off
      - toggle
    mqtt:
      state_topic: zigbee2mqtt/Porch
      command_topic: zigbee2mqtt/Porch/set
```

## Lua Scripting

### Event Script Organization

Scripts are organized by event type:

```
config/events/
├── device/
│   └── <device_id>/
│       ├── <attribute>/
│       │   └── on_change.lua
│       └── actions/
│           └── <action>.lua
├── mqtt/
│   └── <topic>/
│       └── handler.lua
└── time/
    ├── sunrise/
    │   └── handler.lua
    ├── sunset/
    │   └── handler.lua
    ├── every_minute/
    │   └── handler.lua
    ├── every_hour/
    │   └── handler.lua
    └── <HH_MM>/      # Custom times (e.g., 07_00, 17_30)
        └── handler.lua
```

**Note**: Timers created with `timer.after()`, `timer.at()`, or `timer.every()` use callback functions and don't require files.

### Example: Turn on light when switch is pressed

`config/events/device/living_room_switch/action/on_change.lua`:

```lua
local action_value = event.data.action

if action_value == "on" then
    log.info("Switch pressed, turning on lamp")
    device.set("living_room_lamp", {
        state = "ON", 
        brightness = 254
    })
elseif action_value == "brightness_up" then
    local current = device.get("living_room_lamp")
    local new_brightness = math.min(254, (current.brightness or 0) + 50)
    device.set("living_room_lamp", {brightness = new_brightness})
end
```

### Available Lua APIs

#### Device API
```lua
-- Get current device state
local state = device.get("device_id")
-- Returns: {state = "ON", brightness = 200, ...}

-- Set device attributes
device.set("device_id", {state = "ON", brightness = 200})

-- Call device action
device.call("device_id", "toggle", {})
```

#### State API (Persistent Storage)
```lua
-- Get persistent state
local value = state.get("my.key")

-- Set persistent state  
state.set("my.key", {value = 123, timestamp = os.time()})

-- Delete state
state.delete("my.key")
```

#### Log API
```lua
log.info("Information message")
log.warn("Warning message")
log.error("Error message")
```

#### Event Object
```lua
-- Event information
event.source    -- "device", "mqtt", "time", "state"
event.type      -- event type ("state_change", "message", etc.)
event.device    -- device ID (if applicable)
event.attribute -- attribute name (if applicable)
event.topic     -- MQTT topic (if applicable)
event.data      -- event payload (Lua table)
```

### Example Scripts

#### Auto-off after timeout

`config/events/device/porch/state/on_change.lua`:

```lua
local new_state = event.data.state

if new_state == "ON" then
    log.info("Porch light turned ON, will auto-off in 5 minutes")
    
    -- Save turn-on time
    state.set("porch.last_on", os.time())
    
    -- Schedule auto-off (you'd need to implement timer events)
    -- For now, this is handled in a separate time event script
end
```

#### Sync two lights

`config/events/device/living_room_lamp/state/on_change.lua`:

```lua
local new_state = event.data.state

log.info(string.format("Living room lamp: %s", new_state))

-- Sync with bedroom lamp
if new_state == "ON" then
    device.set("bedroom_lamp", {state = "ON"})
else
    device.set("bedroom_lamp", {state = "OFF"})
end
```

#### Work with color (using color helpers)

`config/events/device/living_room_lamp/color/on_change.lua`:

```lua
local new_color = event.data.color

if type(new_color) == "table" and new_color.x and new_color.y then
    -- Convert XY to HSV for easier logic
    local h, s, v = color.xy_to_hsv(new_color.x, new_color.y)
    
    log.info(string.format("Color: hue=%d°, saturation=%d%%", h, s))
    
    -- Detect warm colors
    if h >= 0 and h <= 60 then
        log.info("Warm color detected!")
        -- Copy to another lamp
        device.set("bedroom_lamp", {
            state = "ON",
            color = new_color
        })
    end
end
```

#### Time-based automation

`config/events/time/sunrise/handler.lua`:

```lua
-- Triggered at actual sunrise time (calculated based on your location)
log.info("Sunrise - starting morning routine")

device.set("bedroom_lamp", {state = "ON", brightness = 50})
-- Gradually increase brightness over time if needed
```

`config/events/time/sunset/handler.lua`:

```lua
-- Triggered at actual sunset time (calculated based on your location)
log.info("Sunset - activating outdoor lighting")

device.set("outside_lights", {state = "ON"})
device.set("porch", {state = "ON", brightness = 150})
```

`config/events/time/07_00/handler.lua`:

```lua
-- Morning routine at 07:00 (custom time)
local weekday = tonumber(os.date("%w"))
local is_weekend = (weekday == 0 or weekday == 6)

if not is_weekend then
    log.info("Weekday morning routine")
    device.set("coffee_maker", {state = "ON"})
    device.set("bedroom_lamp", {state = "ON", brightness = 254})
else
    log.info("Weekend - sleeping in!")
end
```

## Lua Helper Libraries

The system includes helper libraries for common tasks:

### Color Helpers (`color`)

Automatically loaded into all scripts. Provides color conversion functions:

- `color.xy_to_rgb(x, y, brightness)` - CIE XY → RGB
- `color.rgb_to_xy(r, g, b)` - RGB → CIE XY
- `color.xy_to_hsv(x, y, brightness)` - CIE XY → HSV
- `color.hsv_to_xy(h, s, v)` - HSV → CIE XY
- `color.rgb_to_hsv(r, g, b)` - RGB → HSV
- `color.hsv_to_rgb(h, s, v)` - HSV → RGB
- `color.describe_color(color_table)` - Human-readable description

See `/config/lib/README.md` for full documentation.

### Timer API (`timer`)

Create dynamic timers with callback functions from Lua scripts:

#### `timer.after(seconds, [id], callback)`
Execute callback after specified seconds. Returns timer ID.
```lua
-- Auto-generate ID
local timer_id = timer.after(300, function()
    device.set("porch", {state = "OFF"})
    log.info("Light turned off after 5 minutes")
end)

-- Custom ID
local timer_id = timer.after(300, "porch_auto_off", function()
    device.set("porch", {state = "OFF"})
end)

-- With closure (capture variables)
local initial_brightness = device.get("bedroom").brightness
local timer_id = timer.after(60, function()
    device.set("bedroom", {
        state = "OFF",
        brightness = initial_brightness
    })
end)
```

#### `timer.at(time, [id], callback)`
Execute callback at specific time (HH:MM format). Returns timer ID.
```lua
local timer_id = timer.at("17:30", function()
    device.set("living_room_lamp", {state = "ON", brightness = 50})
end)

-- With custom ID
local timer_id = timer.at("22:00", "bedtime", function()
    device.set("all_lights", {state = "OFF"})
end)
```

#### `timer.every(seconds, [id], callback)`
Recurring timer with callback. Returns timer ID.
```lua
local timer_id = timer.every(300, function()
    local temp = device.get("temp_sensor").temperature
    if temp > 25 then
        device.set("ac", {state = "ON"})
    end
end)
```

#### `timer.cancel(timer_id)`
Cancel a timer by ID:
```lua
-- Cancel by returned ID
timer.cancel(timer_id)

-- Cancel by custom ID
timer.cancel("porch_auto_off")
```

#### `timer.list()`
Get list of active timer IDs:
```lua
local active_timers = timer.list()
for i, id in ipairs(active_timers) do
    log.info("Active timer: " .. id)
end
```

**Features**:
- ✅ **Closures** - callbacks can access parent scope variables
- ✅ **Auto-ID** - IDs generated automatically or use custom IDs
- ✅ **Returns ID** - all timer functions return timer ID for cancellation
- ✅ **In-memory** - fast execution, no file I/O

## Architecture

```
┌─────────────┐
│   MQTT      │
│  Broker     │
│ (Mosquitto) │
└──────┬──────┘
       │
       │ tcp://host:1883
       ▼
┌─────────────┐
│   MQTT      │
│  Client     │
└──────┬──────┘
       │
       │ Device Events
       ▼
┌─────────────┐     ┌──────────┐
│   Event     │◀────│ Scheduler│
│   Router    │     │(Time Evt)│
└──────┬──────┘     └──────────┘
       │
       │ Find matching scripts
       ▼
┌─────────────┐     ┌──────────┐
│   Worker    │────▶│   Lua    │
│    Pool     │     │ Executor │
└─────────────┘     └────┬─────┘
       │                 │
       │                 │ API calls
       ▼                 ▼
┌─────────────┐     ┌──────────┐
│   Device    │     │  State   │
│  Manager    │     │ Storage  │
└─────────────┘     └──────────┘
```

### Components

- **MQTT Client**: Connects to Mosquitto, subscribes to device topics
- **Scheduler**: Generates time-based events (every minute, hour, sunrise, sunset)
- **Event Router**: Routes events to appropriate Lua scripts based on directory structure
- **Worker Pool**: Executes Lua scripts concurrently with configurable workers
- **Lua Executor**: Runs scripts with API access (device, state, log, color)
- **Device Manager**: Controls devices via MQTT commands
- **State Storage**: Persistent key-value storage using bbolt

## Building

```bash
# Build the binary
go build -o homescript-server ./cmd/server

# Or using make
make build
```

## Commands

### Discovery
```bash
./homescript-server discover [flags]

Flags:
  --mqtt-broker string   MQTT broker URL (default "tcp://localhost:1883")
  --mqtt-user string     MQTT username
  --mqtt-pass string     MQTT password
  --config string        Configuration directory (default "./config")
  --timeout duration     Discovery timeout (default 30s)
  --log-level string     Log level (debug, info, warn, error, critical) (default "error")
```

### Run
```bash
./homescript-server run [flags]

Flags:
  --mqtt-broker string   MQTT broker URL (default "tcp://localhost:1883")
  --mqtt-user string     MQTT username
  --mqtt-pass string     MQTT password
  --config string        Configuration directory (default "./config")
  --db string           Database file path (default "./data/state.db")
  --log-level string    Log level (debug, info, warn, error, critical) (default "error")
  --latitude float      Latitude for sunrise/sunset (auto-detected from IP if 0.0 or omitted)
  --longitude float     Longitude for sunrise/sunset (auto-detected from IP if 0.0 or omitted)
```

**Location Auto-Detection**: If coordinates are not specified (0.0), the server will attempt to detect your location from your public IP address using the free ip-api.com service. This provides approximate coordinates for sunrise/sunset calculations.

## Docker Support

```dockerfile
FROM golang:1.21 as builder
WORKDIR /build
COPY . .
RUN go build -o homescript-server ./cmd/server

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/homescript-server /usr/local/bin/
ENTRYPOINT ["homescript-server"]
CMD ["run"]
```

## Troubleshooting

### Discovery not finding devices

1. Ensure MQTT broker is accessible:
   ```bash
   nc -zv your-mqtt-host 1883
   ```

2. Check if Zigbee2MQTT is publishing devices:
   ```bash
   mosquitto_sub -h your-mqtt-host -t "zigbee2mqtt/bridge/devices"
   ```

3. Enable debug logging in the code (uncomment DEBUG line in `internal/mqtt/client.go`)

### Scripts not executing

1. Check file permissions on Lua scripts
2. Check logs for script errors
3. Verify event routing with log statements

## License

GPL-3.0

