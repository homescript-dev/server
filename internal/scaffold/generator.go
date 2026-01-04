package scaffold

import (
	"fmt"
	"homescript-server/internal/logger"
	"homescript-server/internal/types"
	"os"
	"path/filepath"
	"strings"
)

// GenerateScaffolds creates the directory structure and script templates
func GenerateScaffolds(devices []*types.Device, basePath string) error {
	// Generate helper libraries
	if err := generateHelpers(basePath); err != nil {
		logger.Warn("Failed to generate helper libraries: %v", err)
	}

	// Generate device scaffolds
	for _, dev := range devices {
		if err := generateDeviceScaffold(dev, basePath); err != nil {
			logger.Warn("Failed to generate scaffold for %s: %v", dev.ID, err)
			continue
		}
	}

	// Generate time event scaffolds
	if err := generateTimeScaffolds(basePath); err != nil {
		logger.Warn("Failed to generate time scaffolds: %v", err)
	}

	return nil
}

func generateDeviceScaffold(dev *types.Device, basePath string) error {
	devicePath := filepath.Join(basePath, "events", "device", dev.ID)

	// For Home Assistant devices, use standard attributes for scaffold
	// since HA doesn't provide attribute list in discovery config
	attributes := dev.Attributes
	if strings.HasPrefix(dev.ID, "ha/") && len(attributes) == 0 {
		standardAttrs := getStandardHAAttributes(dev.Type)
		if len(standardAttrs) > 0 {
			attributes = standardAttrs
		}
	}

	// Generate attribute change handlers
	for _, attr := range attributes {
		attrPath := filepath.Join(devicePath, attr)
		if err := os.MkdirAll(attrPath, 0755); err != nil {
			return err
		}

		scriptPath := filepath.Join(attrPath, "on_change.lua")
		if !fileExists(scriptPath) {
			template := generateAttributeScript(dev, attr)
			if err := os.WriteFile(scriptPath, []byte(template), 0644); err != nil {
				return err
			}
			logger.Debug("Created: %s", scriptPath)
		}
	}

	// Generate action handlers
	// For HA devices, use standard actions even if device.Actions is empty
	actions := dev.Actions
	if strings.HasPrefix(dev.ID, "ha/") && len(actions) == 0 {
		standardActions := getStandardHAActions(dev.Type)
		if len(standardActions) > 0 {
			actions = standardActions
		}
	}

	if len(actions) > 0 {
		actionsPath := filepath.Join(devicePath, "actions")
		if err := os.MkdirAll(actionsPath, 0755); err != nil {
			return err
		}

		for _, action := range actions {
			scriptPath := filepath.Join(actionsPath, action+".lua")
			if !fileExists(scriptPath) {
				template := generateActionScript(dev, action)
				if err := os.WriteFile(scriptPath, []byte(template), 0644); err != nil {
					return err
				}
				logger.Debug("Created: %s", scriptPath)
			}
		}
	}

	return nil
}

func generateAttributeScript(dev *types.Device, attr string) string {
	example := ""

	// Special handling for Home Assistant devices (identified by ha/ prefix)
	if strings.HasPrefix(dev.ID, "ha/") {
		switch dev.Type {
		case "climate":
			example = generateClimateExample(dev, attr)
		case "humidifier":
			example = generateHumidifierExample(dev, attr)
		case "vacuum":
			example = generateVacuumExample(dev, attr)
		case "alarm":
			example = generateAlarmExample(dev, attr)
		case "cover":
			example = generateCoverExample(dev, attr)
		case "valve":
			example = generateValveExample(dev, attr)
		case "fan":
			example = generateFanExample(dev, attr)
		case "lock":
			example = generateLockExample(dev, attr)
		case "water_heater":
			example = generateWaterHeaterExample(dev, attr)
		case "lawn_mower":
			example = generateLawnMowerExample(dev, attr)
		case "siren":
			example = generateSirenExample(dev, attr)
		case "number", "select", "text":
			example = generateInputExample(dev, attr)
		case "sensor":
			example = generateSensorExample(dev, attr)
		case "binary_sensor":
			example = generateBinarySensorExample(dev, attr)
		}
	} else if dev.Type == "camera" && dev.Vendor == "Frigate NVR" {
		// Special handling for Frigate cameras
		switch attr {
		case "motion":
			example = `
-- Frigate motion detection
-- Note: Frigate sends "ON"/"OFF" as strings, not booleans
if new_value == "ON" and old_value ~= "ON" then
    log.warn("🚨 Motion detected on ` + dev.Name + `!")
    
    -- Turn on lights or trigger other actions
    -- device.set("porch_light", {state = "ON"})
    
    -- Auto-turn off after 5 minutes
    -- timer.after(300, function()
    --     device.set("porch_light", {state = "OFF"})
    -- end)
end`
		case "detect":
			example = `
-- Object detection state changed
-- Note: Frigate sends "ON"/"OFF" as strings
if new_value == "ON" then
    log.info("Object detection enabled on ` + dev.Name + `")
elseif new_value == "OFF" then
    log.info("Object detection disabled on ` + dev.Name + `")
end`
		case "enabled":
			example = `
-- Camera enabled/disabled
-- Note: Frigate sends "ON"/"OFF" as strings
if new_value == "OFF" and old_value == "ON" then
    log.error("⚠️  Camera ` + dev.Name + ` was DISABLED!")
    -- Alert or enable backup camera
elseif new_value == "ON" and old_value == "OFF" then
    log.info("✅ Camera ` + dev.Name + ` was ENABLED")
end`
		case "recordings":
			example = `
-- Recording state changed
if new_value == "ON" and old_value ~= "ON" then
    log.info("📹 Recording started on ` + dev.Name + `")
elseif new_value == "OFF" and old_value == "ON" then
    log.info("⏹️  Recording stopped on ` + dev.Name + `")
end`
		case "snapshots":
			example = `
-- Snapshot state changed
if new_value == "ON" and old_value ~= "ON" then
    log.info("📸 Snapshots enabled on ` + dev.Name + `")
elseif new_value == "OFF" and old_value == "ON" then
    log.info("⏹️  Snapshots disabled on ` + dev.Name + `")
end`
		case "audio":
			example = `
-- Audio detection state changed
if new_value == "ON" then
    log.info("🔊 Audio detection enabled on ` + dev.Name + `")
else
    log.info("🔇 Audio detection disabled on ` + dev.Name + `")
end`
		case "improve_contrast":
			example = `
-- Improve contrast setting changed
if new_value == "ON" then
    log.info("✨ Contrast improvement enabled on ` + dev.Name + `")
else
    log.info("Contrast improvement disabled on ` + dev.Name + `")
end`
		case "motion_threshold":
			example = `
-- Motion detection threshold changed
-- Value is a number (0-255)
local threshold = tonumber(new_value) or 0
log.info("Motion threshold set to " .. threshold .. " on ` + dev.Name + `")`
		case "motion_contour_area":
			example = `
-- Motion contour area changed
local area = tonumber(new_value) or 0
log.info("Motion contour area set to " .. area .. " on ` + dev.Name + `")`
		case "person", "car", "dog", "cat", "all":
			emoji := map[string]string{
				"person": "👤",
				"car":    "🚗",
				"dog":    "🐕",
				"cat":    "🐈",
				"all":    "🎯",
			}
			icon := emoji[attr]
			example = `
-- Object detection count for ` + attr + `
-- Value is a number (0 = no objects, >0 = objects detected)
local count = tonumber(new_value) or 0
local old_count = tonumber(old_value) or 0

if count > 0 and old_count == 0 then
    log.warn("` + icon + ` ` + strings.Title(attr) + ` detected on ` + dev.Name + ` (count: " .. count .. ")")
    
    -- Example: Turn on lights when person detected
    -- if "` + attr + `" == "person" then
    --     device.set("entrance_light", {state = "ON"})
    -- end
elseif count == 0 and old_count > 0 then
    log.info("` + icon + ` ` + strings.Title(attr) + ` cleared on ` + dev.Name + `")
end`
		default:
			example = `
-- Camera attribute changed
-- Available camera data in event.data:
-- - motion: boolean (motion detected)
-- - objects: array of detected objects
-- - zones: array of active zones
-- - config: camera configuration
--
-- Frigate helper functions available globally:
-- - frigate.has_object(objects, "person")
-- - frigate.filter_by_confidence(objects, 0.8)
-- - frigate.get_objects_in_zone(objects, "zone_name")

log.info("` + attr + ` changed to: " .. tostring(new_value))`
		}
	} else if attr == "color" {
		example = `
-- Color helpers library is available as 'color'
-- Working with color values:
if type(new_value) == "table" then
    -- Describe color in human-readable format
    log.info("Color: " .. color.describe_color(new_value))
    
    -- Convert XY to HSV for easier logic
    if new_value.x and new_value.y then
        local h, s, v = color.xy_to_hsv(new_value.x, new_value.y)
        log.info(string.format("HSV: hue=%d°, sat=%d%%, val=%d%%", h, s, v))
        
        -- Convert to RGB if needed
        -- local r, g, b = color.xy_to_rgb(new_value.x, new_value.y)
    end
    
    -- Copy color to another device
    -- device.set("other_lamp", {color = new_value})
end`
	} else {
		example = `
-- Add your automation logic here
-- Examples:
--
-- if new_value > 25 then
--     device.set("fan.living_room", {state = "on"})
-- end
--
-- if new_value == "off" then
--     log.warn("Device turned off!")
-- end`
	}

	// Template for Home Assistant devices (different structure)
	if strings.HasPrefix(dev.ID, "ha/") {
		return fmt.Sprintf(`-- Device: %s (%s %s)
-- Type: %s
-- Triggered when device state changes via MQTT

-- Home Assistant sends complete JSON state in event.data
-- No need to track individual attributes - work with full state
%s
`, dev.Name, dev.Vendor, dev.Model, dev.Type, example)
	}

	// Normal attribute change event template (for Zigbee, etc)
	return fmt.Sprintf(`-- Device: %s (%s %s)
-- Attribute: %s
-- Triggered when %s changes

local new_value = event.data.%s
local old_value = state.get("device.%s.%s")

-- Save new value to state
state.set("device.%s.%s", new_value)
%s
`, dev.Name, dev.Vendor, dev.Model, attr, attr, attr, dev.ID, attr, dev.ID, attr, example)
}

func generateActionScript(dev *types.Device, action string) string {
	var actionCode string
	var description string

	switch action {
	case "turn_on":
		actionCode = `device.set("` + dev.ID + `", {state = "ON"})`
		description = "Turn on " + dev.ID
	case "turn_off":
		actionCode = `device.set("` + dev.ID + `", {state = "OFF"})`
		description = "Turn off " + dev.ID
	case "toggle":
		actionCode = `local current = device.get("` + dev.ID + `")
    local new_state = (current.state == "ON") and "OFF" or "ON"
    device.set("` + dev.ID + `", {state = new_state})`
		description = "Toggle " + dev.ID + " state"

	// Home Assistant climate actions
	case "set_temperature":
		actionCode = `-- Temperature in Celsius
    local temp = event.data.temperature or 22
    device.set("` + dev.ID + `", {temperature = temp})
    log.info("Set temperature to " .. temp .. "°C")`
		description = "Set target temperature"
	case "set_hvac_mode":
		actionCode = `-- Modes: "off", "heat", "cool", "heat_cool", "auto", "dry", "fan_only"
    local mode = event.data.mode or "heat"
    device.set("` + dev.ID + `", {hvac_mode = mode})
    log.info("Set HVAC mode to " .. mode)`
		description = "Set HVAC mode (heat/cool/off/auto)"
	case "set_fan_mode":
		actionCode = `-- Fan modes: "auto", "low", "medium", "high", etc.
    local fan_mode = event.data.fan_mode or "auto"
    device.set("` + dev.ID + `", {fan_mode = fan_mode})
    log.info("Set fan mode to " .. fan_mode)`
		description = "Set fan mode"
	case "set_preset_mode":
		actionCode = `-- Presets: "eco", "comfort", "away", "sleep", etc.
    local preset = event.data.preset or "comfort"
    device.set("` + dev.ID + `", {preset_mode = preset})
    log.info("Set preset to " .. preset)`
		description = "Set preset mode"

	// HA light actions
	case "set_brightness":
		actionCode = `-- Brightness: 0-255
    local brightness = event.data.brightness or 128
    device.set("` + dev.ID + `", {brightness = brightness})
    log.info("Set brightness to " .. brightness)`
		description = "Set brightness"
	case "set_color":
		actionCode = `-- RGB color: {r=255, g=0, b=0} or {h=120, s=100}
    local color = event.data.color or {r=255, g=255, b=255}
    device.set("` + dev.ID + `", {rgb_color = color})
    log.info("Set color")`
		description = "Set color"

	// HA cover/valve actions
	case "open":
		actionCode = `device.set("` + dev.ID + `", {command = "open"})
    log.info("Opening ` + dev.Name + `")`
		description = "Open cover/valve"
	case "close":
		actionCode = `device.set("` + dev.ID + `", {command = "close"})
    log.info("Closing ` + dev.Name + `")`
		description = "Close cover/valve"
	case "stop":
		actionCode = `device.set("` + dev.ID + `", {command = "stop"})
    log.info("Stopping ` + dev.Name + `")`
		description = "Stop cover/valve"
	case "set_position":
		actionCode = `-- Position: 0-100 (0=closed, 100=open)
    local position = event.data.position or 50
    device.set("` + dev.ID + `", {position = position})
    log.info("Set position to " .. position .. "%")`
		description = "Set position"

	// HA lock actions
	case "lock":
		actionCode = `device.set("` + dev.ID + `", {command = "lock"})
    log.info("Locking ` + dev.Name + `")`
		description = "Lock"
	case "unlock":
		actionCode = `device.set("` + dev.ID + `", {command = "unlock"})
    log.info("Unlocking ` + dev.Name + `")`
		description = "Unlock"

	// HA vacuum actions
	case "start":
		actionCode = `device.set("` + dev.ID + `", {command = "start"})
    log.info("Starting vacuum")`
		description = "Start vacuum"
	case "pause":
		actionCode = `device.set("` + dev.ID + `", {command = "pause"})
    log.info("Pausing")`
		description = "Pause"
	case "return_to_base":
		actionCode = `device.set("` + dev.ID + `", {command = "return_to_base"})
    log.info("Returning to base")`
		description = "Return to base"

	// HA alarm actions
	case "arm_away":
		actionCode = `device.set("` + dev.ID + `", {command = "arm_away"})
    log.info("Arming away")`
		description = "Arm away"
	case "arm_home":
		actionCode = `device.set("` + dev.ID + `", {command = "arm_home"})
    log.info("Arming home")`
		description = "Arm home"
	case "disarm":
		actionCode = `-- May require code
    device.set("` + dev.ID + `", {command = "disarm"})
    log.info("Disarming")`
		description = "Disarm"

	// HA input actions
	case "set_value":
		actionCode = `local value = event.data.value or 0
    device.set("` + dev.ID + `", {value = value})
    log.info("Set value to " .. tostring(value))`
		description = "Set value"
	case "select_option":
		actionCode = `local option = event.data.option or "default"
    device.set("` + dev.ID + `", {option = option})
    log.info("Selected option: " .. option)`
		description = "Select option"

	// HA other actions
	case "set_humidity":
		actionCode = `-- Humidity: 0-100%
    local humidity = event.data.humidity or 60
    device.set("` + dev.ID + `", {humidity = humidity})
    log.info("Set humidity to " .. humidity .. "%")`
		description = "Set humidity"
	case "set_percentage":
		actionCode = `-- Percentage: 0-100
    local percentage = event.data.percentage or 50
    device.set("` + dev.ID + `", {percentage = percentage})
    log.info("Set percentage to " .. percentage .. "%")`
		description = "Set percentage"
	case "set_mode":
		actionCode = `local mode = event.data.mode or "auto"
    device.set("` + dev.ID + `", {mode = mode})
    log.info("Set mode to " .. mode)`
		description = "Set mode"
	case "start_mowing":
		actionCode = `device.set("` + dev.ID + `", {command = "start_mowing"})
    log.info("Starting lawn mower")`
		description = "Start mowing"
	case "dock":
		actionCode = `device.set("` + dev.ID + `", {command = "dock"})
    log.info("Returning to dock")`
		description = "Return to dock"

	// Frigate camera actions - use correct command topics per documentation
	// https://docs.frigate.video/integrations/mqtt/#frigatecamera_namedetectset
	case "enable":
		actionCode = `device.set("` + dev.ID + `", {enabled = "ON"})`
		description = "Enable camera (sends to frigate/{camera}/enabled/set)"
	case "disable":
		actionCode = `device.set("` + dev.ID + `", {enabled = "OFF"})`
		description = "Disable camera (sends to frigate/{camera}/enabled/set)"
	case "detect_on":
		actionCode = `device.set("` + dev.ID + `", {detect = "ON"})`
		description = "Enable object detection (sends to frigate/{camera}/detect/set)"
	case "detect_off":
		actionCode = `device.set("` + dev.ID + `", {detect = "OFF"})`
		description = "Disable object detection (sends to frigate/{camera}/detect/set)"
	case "motion_on":
		actionCode = `device.set("` + dev.ID + `", {motion = "ON"})`
		description = "Enable motion detection (sends to frigate/{camera}/motion/set)"
	case "motion_off":
		actionCode = `device.set("` + dev.ID + `", {motion = "OFF"})`
		description = "Disable motion detection (sends to frigate/{camera}/motion/set)"
	case "recordings_on":
		actionCode = `device.set("` + dev.ID + `", {recordings = "ON"})`
		description = "Enable recordings (sends to frigate/{camera}/recordings/set)"
	case "recordings_off":
		actionCode = `device.set("` + dev.ID + `", {recordings = "OFF"})`
		description = "Disable recordings (sends to frigate/{camera}/recordings/set)"
	case "snapshots_on":
		actionCode = `device.set("` + dev.ID + `", {snapshots = "ON"})`
		description = "Enable snapshots (sends to frigate/{camera}/snapshots/set)"
	case "snapshots_off":
		actionCode = `device.set("` + dev.ID + `", {snapshots = "OFF"})`
		description = "Disable snapshots (sends to frigate/{camera}/snapshots/set)"
	case "improve_contrast":
		actionCode = `-- value should be "ON" or "OFF"
    local value = event.data.value or "ON"
    device.set("` + dev.ID + `", {improve_contrast = value})`
		description = "Set improve contrast (sends to frigate/{camera}/improve_contrast/set)"
	case "ptz_autotracker":
		actionCode = `-- value should be "ON" or "OFF"
    local value = event.data.value or "ON"
    device.set("` + dev.ID + `", {ptz_autotracker = value})`
		description = "Set PTZ autotracker (sends to frigate/{camera}/ptz_autotracker/set)"
	case "motion_threshold":
		actionCode = `-- value should be 0-255
    local value = event.data.value or 25
    device.set("` + dev.ID + `", {motion_threshold = value})`
		description = "Set motion threshold (sends to frigate/{camera}/motion_threshold/set)"
	case "motion_contour_area":
		actionCode = `-- value should be integer
    local value = event.data.value or 10
    device.set("` + dev.ID + `", {motion_contour_area = value})`
		description = "Set motion contour area (sends to frigate/{camera}/motion_contour_area/set)"

	// Generic action - user needs to implement
	default:
		actionCode = `-- TODO: Implement ` + action + ` action
    -- device.set("` + dev.ID + `", {` + action + ` = "value"})`
		description = "Execute " + action
	}

	return fmt.Sprintf(`-- Device: %s (%s %s)
-- Action: %s
-- %s

function %s()
    log.info("Action: %s on %s")
    %s
end

-- If called directly from an event
if event then
    %s()
end

-- Return the function so it can be called from other scripts
return %s
`, dev.Name, dev.Vendor, dev.Model, action, description, action, action, dev.ID, actionCode, action, action)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func generateTimeScaffolds(basePath string) error {
	timeBasePath := filepath.Join(basePath, "events", "time")

	// Create time base directory
	if err := os.MkdirAll(timeBasePath, 0755); err != nil {
		return err
	}

	// Time event types to create
	timeEvents := []struct {
		name        string
		description string
		template    string
	}{
		{
			name:        "sunrise",
			description: "Triggered at sunrise",
			template:    generateSunriseScript(),
		},
		{
			name:        "sunset",
			description: "Triggered at sunset",
			template:    generateSunsetScript(),
		},
		{
			name:        "*_*",
			description: "Wildcard: Every minute",
			template:    generateWildcardEveryMinuteScript(),
		},
		{
			name:        "*_00",
			description: "Wildcard: Every hour at XX:00",
			template:    generateWildcardEveryHourScript(),
		},
		{
			name:        "*_15",
			description: "Wildcard: Every hour at XX:15",
			template:    generateWildcardQuarterScript(),
		},
		{
			name:        "*_30",
			description: "Wildcard: Every hour at XX:30",
			template:    generateWildcardHalfHourScript(),
		},
	}

	for _, timeEvent := range timeEvents {
		eventPath := filepath.Join(timeBasePath, timeEvent.name)
		if err := os.MkdirAll(eventPath, 0755); err != nil {
			return err
		}

		scriptPath := filepath.Join(eventPath, "handler.lua")
		if !fileExists(scriptPath) {
			if err := os.WriteFile(scriptPath, []byte(timeEvent.template), 0644); err != nil {
				return err
			}
			logger.Debug("Created: %s", scriptPath)
		}
	}

	// Create sunrise offset examples
	sunriseOffsets := []struct {
		offset   string
		template string
	}{
		{"-00_30", generateSunriseOffsetScript("-00_30", "30 minutes BEFORE")},
		{"+00_15", generateSunriseOffsetScript("+00_15", "15 minutes AFTER")},
	}

	for _, offset := range sunriseOffsets {
		offsetPath := filepath.Join(timeBasePath, "sunrise", offset.offset)
		if err := os.MkdirAll(offsetPath, 0755); err != nil {
			return err
		}

		scriptPath := filepath.Join(offsetPath, "handler.lua")
		if !fileExists(scriptPath) {
			if err := os.WriteFile(scriptPath, []byte(offset.template), 0644); err != nil {
				return err
			}
			logger.Debug("Created: %s", scriptPath)
		}
	}

	// Create sunset offset examples
	sunsetOffsets := []struct {
		offset   string
		template string
	}{
		{"-00_15", generateSunsetOffsetScript("-00_15", "15 minutes BEFORE")},
		{"+01_30", generateSunsetOffsetScript("+01_30", "1 hour 30 minutes AFTER")},
	}

	for _, offset := range sunsetOffsets {
		offsetPath := filepath.Join(timeBasePath, "sunset", offset.offset)
		if err := os.MkdirAll(offsetPath, 0755); err != nil {
			return err
		}

		scriptPath := filepath.Join(offsetPath, "handler.lua")
		if !fileExists(scriptPath) {
			if err := os.WriteFile(scriptPath, []byte(offset.template), 0644); err != nil {
				return err
			}
			logger.Debug("Created: %s", scriptPath)
		}
	}

	// Create README for time events
	readmePath := filepath.Join(timeBasePath, "README.md")
	if !fileExists(readmePath) {
		if err := os.WriteFile(readmePath, []byte(generateTimeReadme()), 0644); err != nil {
			return err
		}
		logger.Debug("Created: %s", readmePath)
	}

	return nil
}

func generateSunriseScript() string {
	return `-- Time Event: Sunrise
-- Triggered at calculated sunrise time based on your location
-- Time varies by date and location

log.info("Sunrise event triggered")

-- Example: Gradually turn on morning lights
-- device.set("bedroom_lamp", {state = "ON", brightness = 50})
-- device.set("living_room_lamp", {state = "ON", brightness = 100})
`
}

func generateSunsetScript() string {
	return `-- Time Event: Sunset
-- Triggered at calculated sunset time based on your location
-- Time varies by date and location

log.info("Sunset event triggered")

-- Example: Turn on evening lights
-- device.set("outside_lights", {state = "ON"})
-- device.set("living_room_lamp", {state = "ON", brightness = 200})
`
}

func generateEveryMinuteScript() string {
	return `-- Time Event: Every Minute
-- Runs every minute (use sparingly!)

-- Example: Auto-off lights after timeout
-- local porch_on_time = state.get("porch.last_on")
-- if porch_on_time and (os.time() - porch_on_time) > 300 then
--     device.set("porch", {state = "OFF"})
--     state.delete("porch.last_on")
-- end
`
}

func generateEveryHourScript() string {
	return `-- Time Event: Every Hour
-- Runs at XX:00 every hour

log.info("Hour tick: " .. os.date("%H:00"))

-- Example: Adjust heating by time
-- local hour = tonumber(os.date("%H"))
-- if hour >= 22 or hour < 6 then
--     device.set("thermostat", {temperature = 18})
-- end
`
}

func generateTimeReadme() string {
	return `# Time Events

Time-based automation scripts.

## Special Events

- **sunrise/** - Triggered at calculated sunrise time (varies by location and date)
- **sunset/** - Triggered at calculated sunset time (varies by location and date)

## Intervals

- **every_minute/** - Every minute (use sparingly!)
- **every_hour/** - Every hour at XX:00

## Custom Times

Create directories in format ` + "`HH_MM`" + ` for specific times:

- **07_00/** - Every day at 07:00
- **17_30/** - Every day at 17:30
- **22_00/** - Every day at 22:00

Example:
` + "```bash" + `
mkdir -p config/events/time/07_00
cat > config/events/time/07_00/handler.lua << 'EOF'
-- Morning routine at 07:00
local weekday = tonumber(os.date("%w"))
if weekday >= 1 and weekday <= 5 then
    device.set("coffee_maker", {state = "ON"})
end
EOF
` + "```" + `

## Event Data

` + "```lua" + `
event.source = "time"
event.type = "sunrise"  -- or "sunset", "every_hour", "07_00", etc.
event.data = {
    time = 1234567890,  -- Unix timestamp
    hour = 7,
    minute = 0,
    second = 0,
    weekday = 1  -- 0=Sunday, 6=Saturday
}
` + "```" + `

Use standard Lua ` + "`os.date()`" + ` and ` + "`os.time()`" + ` functions for additional time logic.
`
}

func generateWildcardEveryMinuteScript() string {
	return `-- Wildcard: Every minute (*_*)
-- Triggered every minute at second 0
-- Useful for frequent checks or monitoring

log.info("⏱️  Minute tick: " .. os.date("%H:%M:%S"))

-- Example: Check temperature every minute
-- local temp = device.get("temp_sensor").temperature
-- if temp > 30 then
--     log.warn("High temperature: " .. temp)
-- end
`
}

func generateWildcardEveryHourScript() string {
	return `-- Wildcard: Every hour at XX:00 (*_00)
-- Triggered every hour when minute = 0

local hour = tonumber(os.date("%H"))
log.info("🕐 Hour tick: " .. hour .. ":00")

-- Example: Night mode check
-- if hour >= 22 or hour < 6 then
--     log.info("Night mode active")
--     device.set("cameras", {mode = "night"})
-- else
--     log.info("Day mode active")
--     device.set("cameras", {mode = "day"})
-- end
`
}

func generateWildcardQuarterScript() string {
	return `-- Wildcard: Every hour at XX:15 (*_15)
-- Triggered every hour when minute = 15

log.info("🕐 Quarter past: " .. os.date("%H:%M"))

-- Example: Periodic check
-- local status = device.get("system").status
-- log.info("Status check: " .. status)
`
}

func generateWildcardHalfHourScript() string {
	return `-- Wildcard: Every hour at XX:30 (*_30)
-- Triggered every hour when minute = 30

log.info("🕐 Half hour: " .. os.date("%H:%M"))

-- Example: Save energy statistics
-- local power = device.get("meter").power
-- state.set("power.reading_" .. os.time(), power)
`
}

func generateSunriseOffsetScript(offset, description string) string {
	return `-- Sunrise offset: ` + offset + ` (` + description + ` sunrise)
-- Format: -HH_MM (before) or +HH_MM (after)
-- Triggered ` + description + ` calculated sunrise time

log.info("🌅 ` + description + ` sunrise event")

-- Example: Gradual morning wake-up
-- device.set("bedroom_lamp", {state = "ON", brightness = 10})
-- 
-- timer.after(300, function()  -- After 5 minutes
--     device.set("bedroom_lamp", {brightness = 50})
-- end)
`
}

func generateSunsetOffsetScript(offset, description string) string {
	return `-- Sunset offset: ` + offset + ` (` + description + ` sunset)
-- Format: -HH_MM (before) or +HH_MM (after)
-- Triggered ` + description + ` calculated sunset time

log.info("🌆 ` + description + ` sunset event")

-- Example: Pre-sunset preparation
-- device.set("outside_lights", {state = "ON", brightness = 50})
-- 
-- timer.after(600, function()  -- After 10 minutes
--     device.set("outside_lights", {brightness = 255})
-- end)
`
}

func generateHelpers(basePath string) error {
	libPath := filepath.Join(basePath, "lib")
	if err := os.MkdirAll(libPath, 0755); err != nil {
		return err
	}

	// Generate frigate_helpers.lua
	frigateHelperPath := filepath.Join(libPath, "frigate_helpers.lua")
	if !fileExists(frigateHelperPath) {
		if err := os.WriteFile(frigateHelperPath, []byte(getFrigateHelperContent()), 0644); err != nil {
			return err
		}
		logger.Debug("Created: %s", frigateHelperPath)
	}

	// Generate color_helpers.lua
	colorHelperPath := filepath.Join(libPath, "color_helpers.lua")
	if !fileExists(colorHelperPath) {
		if err := os.WriteFile(colorHelperPath, []byte(getColorHelperContent()), 0644); err != nil {
			return err
		}
		logger.Debug("Created: %s", colorHelperPath)
	}

	return nil
}

func getFrigateHelperContent() string {
	return `-- Frigate Camera Helpers
-- Helper functions for working with Frigate NVR camera events

local frigate = {}

-- Parse and log detected objects with details
function frigate.log_objects(objects)
    if not objects or type(objects) ~= "table" then
        return
    end

    for _, obj in ipairs(objects) do
        local info = string.format(
            "Detected: %s (confidence: %.1f%%, area: %d, zone: %s)",
            obj.label or "unknown",
            (obj.score or 0) * 100,
            obj.area or 0,
            obj.current_zones and table.concat(obj.current_zones, ", ") or "none"
        )
        log.info(info)
    end
end

-- Check if specific object type was detected
function frigate.has_object(objects, object_type)
    if not objects or type(objects) ~= "table" then
        return false
    end

    for _, obj in ipairs(objects) do
        if obj.label == object_type then
            return true
        end
    end
    return false
end

-- Get object with highest confidence
function frigate.get_best_object(objects)
    if not objects or type(objects) ~= "table" or #objects == 0 then
        return nil
    end

    local best = objects[1]
    for _, obj in ipairs(objects) do
        if (obj.score or 0) > (best.score or 0) then
            best = obj
        end
    end
    return best
end

-- Filter objects by confidence threshold
function frigate.filter_by_confidence(objects, min_confidence)
    if not objects or type(objects) ~= "table" then
        return {}
    end

    local filtered = {}
    for _, obj in ipairs(objects) do
        if (obj.score or 0) >= min_confidence then
            table.insert(filtered, obj)
        end
    end
    return filtered
end

-- Filter objects by type
function frigate.filter_by_type(objects, object_type)
    if not objects or type(objects) ~= "table" then
        return {}
    end

    local filtered = {}
    for _, obj in ipairs(objects) do
        if obj.label == object_type then
            table.insert(filtered, obj)
        end
    end
    return filtered
end

-- Check if object is in specific zone
function frigate.in_zone(obj, zone_name)
    if not obj or not obj.current_zones then
        return false
    end

    for _, zone in ipairs(obj.current_zones) do
        if zone == zone_name then
            return true
        end
    end
    return false
end

-- Get objects in specific zone
function frigate.get_objects_in_zone(objects, zone_name)
    if not objects or type(objects) ~= "table" then
        return {}
    end

    local filtered = {}
    for _, obj in ipairs(objects) do
        if frigate.in_zone(obj, zone_name) then
            table.insert(filtered, obj)
        end
    end
    return filtered
end

-- Format object info for logging
function frigate.format_object(obj)
    if not obj then
        return "nil"
    end

    local parts = {}
    table.insert(parts, obj.label or "unknown")

    if obj.score then
        table.insert(parts, string.format("%.1f%%", obj.score * 100))
    end

    if obj.area then
        table.insert(parts, string.format("area=%d", obj.area))
    end

    if obj.current_zones and #obj.current_zones > 0 then
        table.insert(parts, "zones=[" .. table.concat(obj.current_zones, ",") .. "]")
    end

    return table.concat(parts, " ")
end

-- Describe camera activity in human-readable format
function frigate.describe_activity(data)
    local parts = {}

    if data.motion == true then
        table.insert(parts, "motion detected")
    end

    if data.objects and #data.objects > 0 then
        local obj_summary = {}
        for _, obj in ipairs(data.objects) do
            table.insert(obj_summary, obj.label or "unknown")
        end
        table.insert(parts, string.format("%d object(s): %s",
            #data.objects, table.concat(obj_summary, ", ")))
    end

    if #parts == 0 then
        return "no activity"
    end

    return table.concat(parts, "; ")
end

-- Check if this is a new detection
function frigate.is_new_detection(new_objects, old_objects)
    if not new_objects or #new_objects == 0 then
        return false
    end

    if not old_objects or #old_objects == 0 then
        return true
    end

    return #new_objects > #old_objects
end

-- Get camera activity summary for state tracking
function frigate.get_activity_summary(data)
    local summary = {
        timestamp = os.time(),
        motion = data.motion or false,
        object_count = (data.objects and #data.objects) or 0,
        objects = {}
    }

    if data.objects then
        for _, obj in ipairs(data.objects) do
            table.insert(summary.objects, {
                label = obj.label,
                score = obj.score,
                zones = obj.current_zones
            })
        end
    end

    return summary
end

return frigate
`
}

func getColorHelperContent() string {
	return `-- Color Helper Library
-- Helper functions for working with color values in various formats

local color = {}

-- Convert XY to RGB
function color.xy_to_rgb(x, y, brightness)
    brightness = brightness or 255
    local z = 1.0 - x - y
    local Y = brightness / 255.0
    local X = (Y / y) * x
    local Z = (Y / y) * z

    local r = X * 1.656492 - Y * 0.354851 - Z * 0.255038
    local g = -X * 0.707196 + Y * 1.655397 + Z * 0.036152
    local b = X * 0.051713 - Y * 0.121364 + Z * 1.011530

    r = r <= 0.0031308 and 12.92 * r or (1.0 + 0.055) * math.pow(r, (1.0 / 2.4)) - 0.055
    g = g <= 0.0031308 and 12.92 * g or (1.0 + 0.055) * math.pow(g, (1.0 / 2.4)) - 0.055
    b = b <= 0.0031308 and 12.92 * b or (1.0 + 0.055) * math.pow(b, (1.0 / 2.4)) - 0.055

    local function clamp(v)
        return math.max(0, math.min(255, math.floor(v * 255 + 0.5)))
    end

    return clamp(r), clamp(g), clamp(b)
end

-- Convert XY to HSV
function color.xy_to_hsv(x, y)
    local r, g, b = color.xy_to_rgb(x, y, 255)
    r, g, b = r / 255, g / 255, b / 255

    local max = math.max(r, g, b)
    local min = math.min(r, g, b)
    local delta = max - min

    local h, s, v = 0, 0, max

    if delta > 0 then
        s = delta / max

        if r == max then
            h = ((g - b) / delta) % 6
        elseif g == max then
            h = (b - r) / delta + 2
        else
            h = (r - g) / delta + 4
        end

        h = h * 60
        if h < 0 then h = h + 360 end
    end

    return math.floor(h + 0.5), math.floor(s * 100 + 0.5), math.floor(v * 100 + 0.5)
end

-- Describe color in human-readable format
function color.describe_color(color_value)
    if type(color_value) ~= "table" then
        return tostring(color_value)
    end

    if color_value.x and color_value.y then
        local h, s, v = color.xy_to_hsv(color_value.x, color_value.y)
        return string.format("HSV(h=%d°, s=%d%%, v=%d%%)", h, s, v)
    elseif color_value.hue and color_value.saturation then
        return string.format("HS(h=%d, s=%d)", color_value.hue or 0, color_value.saturation or 0)
    end

    return "Unknown color format"
end

return color
`
}
