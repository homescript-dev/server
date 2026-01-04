package scaffold

import (
	"homescript-server/internal/types"
	"strings"
)

// Frigate camera example generators

func generateFrigateMotionExample(dev *types.Device) string {
	return `
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
}

func generateFrigateDetectExample(dev *types.Device) string {
	return `
-- Object detection state changed
-- Note: Frigate sends "ON"/"OFF" as strings
if new_value == "ON" then
    log.info("Object detection enabled on ` + dev.Name + `")
elseif new_value == "OFF" then
    log.info("Object detection disabled on ` + dev.Name + `")
end`
}

func generateFrigateEnabledExample(dev *types.Device) string {
	return `
-- Camera enabled/disabled
-- Note: Frigate sends "ON"/"OFF" as strings
if new_value == "OFF" and old_value == "ON" then
    log.error("⚠️  Camera ` + dev.Name + ` was DISABLED!")
    -- Alert or enable backup camera
elseif new_value == "ON" and old_value == "OFF" then
    log.info("✅ Camera ` + dev.Name + ` was ENABLED")
end`
}

func generateFrigateRecordingExample(dev *types.Device) string {
	return `
-- Recording state changed
if new_value == "ON" and old_value ~= "ON" then
    log.info("📹 Recording started on ` + dev.Name + `")
elseif new_value == "OFF" and old_value == "ON" then
    log.info("⏹️  Recording stopped on ` + dev.Name + `")
end`
}

func generateFrigateSnapshotExample(dev *types.Device) string {
	return `
-- Snapshot state changed
if new_value == "ON" and old_value ~= "ON" then
    log.info("📸 Snapshots enabled on ` + dev.Name + `")
elseif new_value == "OFF" and old_value == "ON" then
    log.info("⏹️  Snapshots disabled on ` + dev.Name + `")
end`
}

func generateFrigateAudioExample(dev *types.Device) string {
	return `
-- Audio detection state changed
if new_value == "ON" then
    log.info("🔊 Audio detection enabled on ` + dev.Name + `")
else
    log.info("🔇 Audio detection disabled on ` + dev.Name + `")
end`
}

func generateFrigateContrastExample(dev *types.Device) string {
	return `
-- Improve contrast setting changed
if new_value == "ON" then
    log.info("✨ Contrast improvement enabled on ` + dev.Name + `")
else
    log.info("Contrast improvement disabled on ` + dev.Name + `")
end`
}

func generateFrigateThresholdExample(dev *types.Device) string {
	return `
-- Motion detection threshold changed
-- Value is a number (0-255)
local threshold = tonumber(new_value) or 0
log.info("Motion threshold set to " .. threshold .. " on ` + dev.Name + `")`
}

func generateFrigateContourExample(dev *types.Device) string {
	return `
-- Motion contour area changed
local area = tonumber(new_value) or 0
log.info("Motion contour area set to " .. area .. " on ` + dev.Name + `")`
}

func generateFrigateObjectExample(dev *types.Device, attr string) string {
	emoji := map[string]string{
		"person": "👤",
		"car":    "🚗",
		"dog":    "🐕",
		"cat":    "🐈",
		"all":    "🎯",
	}
	icon := emoji[attr]
	return `
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
}

func generateFrigateDefaultExample(dev *types.Device) string {
	return `
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

log.info("` + dev.Name + ` attribute changed to: " .. tostring(new_value))`
}
