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

	// Generate attribute change handlers
	for _, attr := range dev.Attributes {
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
	if len(dev.Actions) > 0 {
		actionsPath := filepath.Join(devicePath, "actions")
		if err := os.MkdirAll(actionsPath, 0755); err != nil {
			return err
		}

		for _, action := range dev.Actions {
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

	// Special handling for Frigate cameras
	if dev.Type == "camera" && dev.Vendor == "Frigate NVR" {
		switch attr {
		case "motion":
			example = `
-- Frigate motion detection
if new_value == true and old_value ~= true then
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
if new_value == true then
    log.info("Object detection enabled on ` + dev.Name + `")
else
    log.info("Object detection disabled on ` + dev.Name + `")
end

-- Note: Actual object detections come through event.data.objects
-- Frigate helper is available globally (no require needed)
-- Check if specific objects were detected:
if event.data.objects then
    for _, obj in ipairs(event.data.objects) do
        log.info(string.format("Detected: %s (confidence: %.1f%%)", obj.label, obj.score * 100))
        
        -- React to specific objects
        if obj.label == "person" then
            log.warn("👤 Person detected!")
            -- device.set("entrance_light", {state = "ON"})
        elseif obj.label == "car" then
            log.info("🚗 Car detected")
        end
    end
    
    -- Or use frigate helper functions:
    -- if frigate.has_object(event.data.objects, "person") then
    --     log.warn("👤 Person detected!")
    -- end
end`
		case "enabled":
			example = `
-- Camera enabled/disabled
if new_value == false and old_value == true then
    log.error("⚠️  Camera ` + dev.Name + ` was DISABLED!")
    -- Alert or enable backup camera
elseif new_value == true and old_value == false then
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
		case "person", "car", "dog", "cat":
			emoji := map[string]string{
				"person": "👤",
				"car":    "🚗",
				"dog":    "🐕",
				"cat":    "🐈",
			}
			icon := emoji[attr]
			if icon == "" {
				icon = "📸"
			}
			example = `
-- Snapshot received when ` + attr + ` was detected
-- event.data contains:
--   object_type: "` + attr + `"
--   snapshot: JPEG image data (byte array)
--   size: size in bytes
--
-- This event is triggered when Frigate detects a ` + attr + ` and takes a snapshot

if event.data.snapshot then
    log.warn("` + icon + ` ` + strings.Title(attr) + ` detected on ` + dev.Name + ` (snapshot: " .. event.data.size .. " bytes)")
    
    -- Example: Save snapshot to file
    -- local filename = string.format("/tmp/` + attr + `_%s_%d.jpg", "` + dev.ID + `", os.time())
    -- local file = io.open(filename, "wb")
    -- if file then
    --     file:write(event.data.snapshot)
    --     file:close()
    --     log.info("Saved snapshot to: " .. filename)
    -- end
    
    -- Example: Turn on lights when person detected
    -- if event.data.object_type == "person" then
    --     device.set("entrance_light", {state = "ON"})
    --     timer.after(300, function()
    --         device.set("entrance_light", {state = "OFF"})
    --     end)
    -- end
    
    -- Save detection time to state
    state.set("` + dev.ID + `.last_` + attr + `_detection", os.time())
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

	// Special template for snapshot events (person, car, dog, cat)
	isSnapshot := dev.Type == "camera" && dev.Vendor == "Frigate NVR" &&
		(attr == "person" || attr == "car" || attr == "dog" || attr == "cat")

	if isSnapshot {
		return fmt.Sprintf(`-- Device: %s (%s %s)
-- Snapshot Event: %s
-- Triggered when %s is detected and snapshot is captured

%s
`, dev.Name, dev.Vendor, dev.Model, attr, attr, example)
	}

	// Normal attribute change event template
	return fmt.Sprintf(`-- Device: %s (%s %s)
-- Attribute: %s
-- Triggered when %s changes

local new_value = event.data.%s
local old_value = state.get("device.%s.%s")

log.info(string.format("%s.%s changed from %%s to %%s", tostring(old_value), tostring(new_value)))

-- Save new value to state
state.set("device.%s.%s", new_value)
%s
`, dev.Name, dev.Vendor, dev.Model, attr, attr, attr, dev.ID, attr, dev.ID, attr, dev.ID, attr, example)
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

	// Frigate camera actions
	case "enable":
		actionCode = `device.set("` + dev.ID + `", {enabled = "ON"})`
		description = "Enable camera"
	case "disable":
		actionCode = `device.set("` + dev.ID + `", {enabled = "OFF"})`
		description = "Disable camera"
	case "start_detect":
		actionCode = `device.set("` + dev.ID + `", {detect = "ON"})`
		description = "Start object detection"
	case "stop_detect":
		actionCode = `device.set("` + dev.ID + `", {detect = "OFF"})`
		description = "Stop object detection"
	case "start_recordings":
		actionCode = `device.set("` + dev.ID + `", {recordings = "ON"})`
		description = "Start recordings"
	case "stop_recordings":
		actionCode = `device.set("` + dev.ID + `", {recordings = "OFF"})`
		description = "Stop recordings"
	case "start_snapshots":
		actionCode = `device.set("` + dev.ID + `", {snapshots = "ON"})`
		description = "Start snapshots"
	case "stop_snapshots":
		actionCode = `device.set("` + dev.ID + `", {snapshots = "OFF"})`
		description = "Stop snapshots"

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
