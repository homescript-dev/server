package scaffold

import (
	"homescript-server/internal/types"
)

// Home Assistant device example generators

func generateClimateExample(dev *types.Device, attr string) string {
	switch attr {
	case "current_temperature":
		return `
-- Current temperature monitoring
local temp = tonumber(event.data.current_temperature)
if temp then
    log.info("` + dev.Name + ` temperature: " .. temp .. "°C")
    
    -- Example: Alert if too hot/cold
    -- if temp > 28 then
    --     log.warn("Temperature too high!")
    -- elseif temp < 18 then
    --     log.warn("Temperature too low!")
    -- end
end`
	case "target_temperature":
		return `
-- Target temperature changed
local target = tonumber(event.data.target_temperature or event.data.temperature)
if target then
    log.info("` + dev.Name + ` target set to: " .. target .. "°C")
end

-- Control: Set target temperature
-- device.set("` + dev.ID + `", {temperature = 22})`
	case "hvac_mode":
		return `
-- HVAC mode changed (off, heat, cool, auto, etc.)
local mode = event.data.hvac_mode
if mode then
    log.info("` + dev.Name + ` mode: " .. mode)
    
    -- Example automation
    -- if mode == "off" then
    --     log.info("Climate turned off")
    -- elseif mode == "heat" then
    --     log.info("Heating active")
    -- end
end

-- Control: Change mode
-- device.set("` + dev.ID + `", {hvac_mode = "heat"})`
	case "hvac_action":
		return `
-- Current action (heating, cooling, idle, etc.)
local action = event.data.hvac_action or event.data.action
if action then
    log.info("` + dev.Name + ` action: " .. action)
end`
	case "fan_mode":
		return `
-- Fan mode changed
local fan_mode = event.data.fan_mode
if fan_mode then
    log.info("` + dev.Name + ` fan mode: " .. fan_mode)
end

-- Control: Set fan mode
-- device.set("` + dev.ID + `", {fan_mode = "auto"})`
	case "preset_mode":
		return `
-- Preset mode changed (comfort, eco, away, etc.)
local preset = event.data.preset_mode
if preset then
    log.info("` + dev.Name + ` preset: " .. preset)
end

-- Control: Set preset
-- device.set("` + dev.ID + `", {preset_mode = "eco"})`
	default:
		return `
-- Climate device state
-- Access: event.data.current_temperature, event.data.hvac_mode, etc.
log.info("` + dev.Name + ` state updated")`
	}
}

func generateHumidifierExample(dev *types.Device, attr string) string {
	return `
-- Humidifier control
local value = tonumber(event.data.current_humidity or event.data.target_humidity or event.data.humidity)
if value then
    log.info("` + dev.Name + ` humidity: " .. value .. "%")
end

-- Control:
-- device.set("` + dev.ID + `", {humidity = 60})
-- device.set("` + dev.ID + `", {mode = "auto"})`
}

func generateVacuumExample(dev *types.Device, attr string) string {
	return `
-- Robot vacuum status
local status = event.data.state or event.data.status
log.info("` + dev.Name + ` status: " .. tostring(status))

-- Control:
-- device.set("` + dev.ID + `", {command = "start"})
-- device.set("` + dev.ID + `", {command = "return_to_base"})`
}

func generateAlarmExample(dev *types.Device, attr string) string {
	return `
-- Alarm system state
local state = event.data.state
log.info("` + dev.Name + ` state: " .. tostring(state))

-- Control:
-- device.set("` + dev.ID + `", {command = "arm_away"})
-- device.set("` + dev.ID + `", {command = "disarm"})`
}

func generateCoverExample(dev *types.Device, attr string) string {
	return `
-- Cover/blind control
local pos = tonumber(event.data.position)
if pos then
    log.info("` + dev.Name + ` position: " .. pos .. "%")
end

-- Control:
-- device.set("` + dev.ID + `", {command = "open"})
-- device.set("` + dev.ID + `", {position = 50})`
}

func generateValveExample(dev *types.Device, attr string) string {
	return `
-- Valve control
local pos = tonumber(event.data.position)
if pos then
    log.info("` + dev.Name + ` position: " .. pos .. "%")
end

-- Control:
-- device.set("` + dev.ID + `", {command = "open"})`
}

func generateFanExample(dev *types.Device, attr string) string {
	return `
-- Fan control
log.info("` + dev.Name + ` state: " .. tostring(event.data.state))

-- Control:
-- device.set("` + dev.ID + `", {percentage = 75})
-- device.set("` + dev.ID + `", {oscillate = true})`
}

func generateLockExample(dev *types.Device, attr string) string {
	return `
-- Lock status
log.info("` + dev.Name + ` state: " .. tostring(event.data.state))

-- Control:
-- device.set("` + dev.ID + `", {command = "lock"})`
}

func generateWaterHeaterExample(dev *types.Device, attr string) string {
	return `
-- Water heater control
local temp = tonumber(event.data.current_temperature or event.data.temperature)
if temp then
    log.info("` + dev.Name + ` temperature: " .. temp .. "°C")
end

-- Control:
-- device.set("` + dev.ID + `", {temperature = 60})`
}

func generateLawnMowerExample(dev *types.Device, attr string) string {
	return `
-- Lawn mower control
log.info("` + dev.Name + ` status: " .. tostring(event.data.state))

-- Control:
-- device.set("` + dev.ID + `", {command = "start_mowing"})`
}

func generateSirenExample(dev *types.Device, attr string) string {
	return `
-- Siren control
log.info("` + dev.Name + ` state: " .. tostring(event.data.state))

-- Control:
-- device.set("` + dev.ID + `", {command = "turn_on"})`
}

func generateInputExample(dev *types.Device, attr string) string {
	return `
-- Input control
log.info("` + dev.Name + ` value: " .. tostring(event.data.value or event.data.state))

-- Control:
-- device.set("` + dev.ID + `", {value = 42})`
}

func generateSensorExample(dev *types.Device, attr string) string {
	return `
-- Sensor device (read-only)
-- Sensors send their state in event.data

-- Get sensor value
local value = event.data.state or event.data.value

if value then
    log.info("` + dev.Name + ` value: " .. tostring(value))
    
    -- Example: Alert on threshold
    -- local num_value = tonumber(value)
    -- if num_value and num_value > 30 then
    --     log.warn("High value detected: " .. num_value)
    --     -- Trigger action
    --     device.set("fan", {state = "ON"})
    -- end
end

-- Access other attributes if present
if event.data.temperature then
    log.info("Temperature: " .. tostring(event.data.temperature))
end

if event.data.humidity then
    log.info("Humidity: " .. tostring(event.data.humidity))
end

-- Save to state for historical tracking
-- state.set("` + dev.ID + `.last_reading", {
--     value = value,
--     timestamp = os.time()
-- })`
}

func generateBinarySensorExample(dev *types.Device, attr string) string {
	return `
-- Binary sensor device (ON/OFF states)
-- Reports state as "ON" or "OFF" (or true/false)

local state = event.data.state or event.data.value

log.info("` + dev.Name + ` state: " .. tostring(state))

-- Handle state changes
if state == "ON" or state == true then
    log.info("` + dev.Name + ` is active")
    
    -- Example: Trigger automation
    -- if "` + dev.ID + `" == "ha/motion_sensor" then
    --     device.set("lights", {state = "ON"})
    --     
    --     -- Auto-off after 5 minutes
    --     timer.after(300, function()
    --         device.set("lights", {state = "OFF"})
    --     end)
    -- end
    
elseif state == "OFF" or state == false then
    log.info("` + dev.Name + ` is inactive")
end

-- Track state changes
-- local old_state = state.get("` + dev.ID + `.state")
-- if old_state ~= state then
--     log.info("State changed from " .. tostring(old_state) .. " to " .. tostring(state))
--     state.set("` + dev.ID + `.state", state)
--     state.set("` + dev.ID + `.last_change", os.time())
-- end`
}
